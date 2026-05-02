// Package postgrestest は DB を用いるテスト全般のヘルパを提供する。
package postgrestest

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Postgres は起動済み PostgreSQL コンテナと接続プールのハンドル。
type Postgres struct {
	Container *postgres.PostgresContainer
	Pool      *pgxpool.Pool
	schemas   []string
}

type config struct {
	schemaFiles []string
	schemas     []string
	searchPath  []string
}

// Option は Start の振る舞いを構成する。
type Option func(*config)

// WithSchemaFile は repo-root 起点の相対パスでスキーマ SQL を登録する。
func WithSchemaFile(repoRelativePath string) Option {
	return func(c *config) {
		c.schemaFiles = append(c.schemaFiles, repoRelativePath)
	}
}

// WithSchema は Truncate 対象スキーマを登録する。複数指定可。
func WithSchema(name string) Option {
	return func(c *config) {
		c.schemas = append(c.schemas, name)
	}
}

// WithSearchPath は pool 全体に適用する search_path を登録する。
func WithSearchPath(schemas ...string) Option {
	return func(c *config) {
		c.searchPath = append(c.searchPath, schemas...)
	}
}

// Start は postgres:16-alpine コンテナを起動し、登録された schema SQL を適用したうえで接続プールを返す。
func Start(ctx context.Context, opts ...Option) (*Postgres, error) {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}
	if len(cfg.schemaFiles) == 0 {
		return nil, errors.New("postgrestest: at least one WithSchemaFile is required")
	}
	if len(cfg.schemas) == 0 {
		return nil, errors.New("postgrestest: at least one WithSchema is required")
	}

	root, err := repoRoot()
	if err != nil {
		return nil, err
	}

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres container: %w", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, errors.Join(fmt.Errorf("container connection string: %w", err), container.Terminate(ctx))
	}

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("parse pool config: %w", err), container.Terminate(ctx))
	}
	// Why: ALTER DATABASE / ALTER ROLE は既存接続には反映されないため、AfterConnect で毎接続にセットする。
	if len(cfg.searchPath) > 0 {
		setStmt := "SET search_path TO " + strings.Join(quoteIdents(cfg.searchPath), ", ")
		poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			_, execErr := conn.Exec(ctx, setStmt)
			return execErr
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("pgxpool new: %w", err), container.Terminate(ctx))
	}

	for _, rel := range cfg.schemaFiles {
		path := filepath.Join(root, rel)
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			pool.Close()
			return nil, errors.Join(fmt.Errorf("read schema %s: %w", path, rerr), container.Terminate(ctx))
		}
		if _, eerr := pool.Exec(ctx, string(data)); eerr != nil {
			pool.Close()
			return nil, errors.Join(fmt.Errorf("apply schema %s: %w", path, eerr), container.Terminate(ctx))
		}
	}

	return &Postgres{Container: container, Pool: pool, schemas: cfg.schemas}, nil
}

// Close は pool をクローズしコンテナを終了する。両方のエラーを集約して返す。
func (p *Postgres) Close(ctx context.Context) error {
	p.Pool.Close()
	if err := p.Container.Terminate(ctx); err != nil {
		return fmt.Errorf("terminate container: %w", err)
	}
	return nil
}

// Truncate は登録 schema 配下の全 BASE TABLE を動的に列挙して TRUNCATE する。
func (p *Postgres) Truncate(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	rows, err := p.Pool.Query(ctx, `
		SELECT table_schema || '.' || table_name
		  FROM information_schema.tables
		 WHERE table_schema = ANY($1)
		   AND table_type = 'BASE TABLE'
	`, p.schemas)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if serr := rows.Scan(&name); serr != nil {
			t.Fatalf("scan table name: %v", serr)
		}
		tables = append(tables, name)
	}
	if rerr := rows.Err(); rerr != nil {
		t.Fatalf("iterate tables: %v", rerr)
	}
	if len(tables) == 0 {
		return
	}

	stmt := "TRUNCATE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE"
	if _, err := p.Pool.Exec(ctx, stmt); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// RunMain は TestMain のボイラープレートを集約する (コンテナ起動 → m.Run → クリーンアップ)。
func RunMain(m *testing.M, out **Postgres, opts ...Option) int {
	ctx := context.Background()

	pg, err := Start(ctx, opts...)
	if err != nil {
		log.Fatalf("postgrestest.Start: %v", err)
	}
	*out = pg

	defer func() {
		if cerr := pg.Close(ctx); cerr != nil {
			log.Printf("postgrestest.Close: %v", cerr)
		}
	}()

	return m.Run()
}

// repoRoot は本ファイルの位置から go.mod を持つディレクトリを探索して返す。
func repoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found from postgrestest")
		}
		dir = parent
	}
}

// quoteIdents は identifier を二重引用符でエスケープする。
func quoteIdents(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = `"` + strings.ReplaceAll(n, `"`, `""`) + `"`
	}
	return out
}
