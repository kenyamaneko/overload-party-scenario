package postgres_test

import (
	"os"
	"testing"

	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres/postgrestest"
)

var sharedPg *postgrestest.Postgres

// TestMain は scenario schema + account stub schema を適用した Postgres コンテナを
// テストパッケージ共有で 1 回だけ起動する。
//
// scenario の本番 DDL (db/schema.sql) に加えて、cross-schema 参照する
// players / player_factions を scenario schema 内にテスト専用スタブとして作成する。
// これはスキーマ分割完了までの暫定対応（internal/repository/postgres/story_repo.go の
// GetUnlockContext コメント参照）。
//
// StoryRepository は unqualified 参照 (scenario.scenario_episodes ではなく
// scenario_episodes) で書かれているため search_path に scenario を通す。
// 本番ではユーザ名 `scenario` により $user 解決で同じ効果を得ている。
func TestMain(m *testing.M) {
	os.Exit(postgrestest.RunMain(m, &sharedPg,
		postgrestest.WithSchemaFile("db/schema.sql"),
		postgrestest.WithSchemaFile("internal/repository/postgres/testdata/account_stub.sql"),
		postgrestest.WithSchema("scenario"),
		postgrestest.WithSearchPath("scenario", "public"),
	))
}
