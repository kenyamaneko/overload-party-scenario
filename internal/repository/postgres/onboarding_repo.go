package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

// pgCodeUniqueViolation は PostgreSQL の一意制約違反を表す SQLSTATE。
// scenario.player_onboarding の PRIMARY KEY (player_id) 違反をここで検出し、
// port.ErrAlreadyOnboarded に翻訳することで usecase 層から pgx/pgconn を隠蔽する。
const pgCodeUniqueViolation = "23505"

var _ port.OnboardingRepo = (*OnboardingRepository)(nil)

// OnboardingRepository は PostgreSQL で OnboardingRepo を実装する。
type OnboardingRepository struct {
	pool *pgxpool.Pool
}

// NewOnboardingRepository は pgxpool.Pool を受け取り OnboardingRepository を構築する。
func NewOnboardingRepository(pool *pgxpool.Pool) *OnboardingRepository {
	return &OnboardingRepository{pool: pool}
}

// MarkComplete はビジネス行 (scenario.player_onboarding への INSERT) と
// outbox イベント行の INSERT を同一トランザクションで実行する。
// 一意制約違反は port.ErrAlreadyOnboarded に classify する。
// events は順に outbox に積み、いずれかが失敗すれば tx 全体を rollback する。
func (r *OnboardingRepository) MarkComplete(ctx context.Context, playerID string, events ...port.OutboxEvent) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// Why: defer rollback は Commit 成功後に呼ばれても no-op なので安全。
	// 途中 return の場合に必ず rollback させるためにここで仕掛ける。
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO scenario.player_onboarding (player_id)
		 VALUES ($1)`,
		playerID,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgCodeUniqueViolation {
			return port.ErrAlreadyOnboarded
		}
		return fmt.Errorf("insert player_onboarding: %w", err)
	}

	for _, ev := range events {
		if err := writeOutboxEvent(ctx, tx, ev); err != nil {
			return fmt.Errorf("write outbox event: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// PublishEvents は outbox 行のみを単一トランザクションで INSERT する。
// scenario 側にビジネス書き込みを伴わないステップ (name-set / faction-set など) で、
// 同一 publish 単位の atomic enqueue を保証する。events が空なら no-op。
func (r *OnboardingRepository) PublishEvents(ctx context.Context, events ...port.OutboxEvent) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, ev := range events {
		if err := writeOutboxEvent(ctx, tx, ev); err != nil {
			return fmt.Errorf("write outbox event: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
