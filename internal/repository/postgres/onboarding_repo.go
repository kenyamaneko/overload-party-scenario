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

// MarkComplete は player_onboarding 行と outbox イベント行の INSERT を同一トランザクションで実行する。
func (r *OnboardingRepository) MarkComplete(ctx context.Context, playerID string, events ...port.OutboxEvent) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
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

// PublishEvents は outbox 行のみを単一トランザクションで INSERT する (events が空なら no-op)。
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
