//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres"
)

// newPlayerID は UUID 形式の一意な player_id を組み立てる。
// player_id は scenario.player_onboarding の UUID 型カラムへ入るため、形式が重要。
// テスト間で一意になるよう testIdx / seedIdx を埋め込む。
func newPlayerID(testIdx, seedIdx int) string {
	return fmt.Sprintf("%08d-%04d-%04d-0000-000000000000", testIdx+1, seedIdx, seedIdx)
}

// insertOnboardingRow は scenario.player_onboarding に既完了レコードを直接 INSERT する
// (MarkComplete を通らずに状態だけ作る seed)。
func insertOnboardingRow(t *testing.T, playerID string) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.player_onboarding (player_id) VALUES ($1)`,
		playerID)
	require.NoError(t, err)
}

func TestPublishEvents(t *testing.T) {
	repo := postgres.NewOnboardingRepository(sharedPg.Pool)
	ctx := context.Background()

	makeEvents := func(n int) []port.OutboxEvent {
		out := make([]port.OutboxEvent, 0, n)
		for i := range n {
			out = append(out, port.OutboxEvent{
				EventID:   uuid.New(),
				EventType: fmt.Sprintf("publish-test-event-%d", i),
				Payload:   []byte(fmt.Sprintf(`{"i":%d}`, i)),
			})
		}
		return out
	}

	t.Run("outbox への複数イベント投入", func(t *testing.T) {
		tests := []struct {
			name            string
			events          []port.OutboxEvent
			wantOutboxCount int
		}{
			{
				name:            "events が複数のとき、単一 tx で全行が outbox に積まれる",
				events:          makeEvents(2),
				wantOutboxCount: 2,
			},
			{
				name:            "events が 1 件のとき、outbox に 1 行積む",
				events:          makeEvents(1),
				wantOutboxCount: 1,
			},
			{
				name:            "events が 0 件のとき、outbox に何も積まない",
				events:          nil,
				wantOutboxCount: 0,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)

				require.NoError(t, repo.PublishEvents(ctx, tt.events...))

				var outboxCount int
				require.NoError(t, sharedPg.Pool.QueryRow(ctx,
					`SELECT COUNT(*) FROM scenario.outbox_events`).Scan(&outboxCount))
				assert.Equal(t, tt.wantOutboxCount, outboxCount)
			})
		}
	})
}

func TestOnboardingRepository_MarkComplete(t *testing.T) {
	repo := postgres.NewOnboardingRepository(sharedPg.Pool)
	ctx := context.Background()

	// makeEvents は任意件数の OutboxEvent を作るヘルパ。event_type / payload は任意で良い
	// (scenario.outbox_events に CHECK 制約は無い)。
	makeEvents := func(n int) []port.OutboxEvent {
		out := make([]port.OutboxEvent, 0, n)
		for i := range n {
			out = append(out, port.OutboxEvent{
				EventID:   uuid.New(),
				EventType: fmt.Sprintf("test-event-type-%d", i),
				Payload:   []byte(fmt.Sprintf(`{"i":%d}`, i)),
			})
		}
		return out
	}

	t.Run("オンボーディング完了の記録", func(t *testing.T) {
		tests := []struct {
			name                string
			seed                func(t *testing.T, playerID string)
			events              []port.OutboxEvent
			wantErr             error
			wantOutboxCount     int
			wantOnboardingCount int
		}{
			{
				name:                "events が複数でも、player_onboarding と outbox へ atomic に書き込む",
				seed:                func(t *testing.T, playerID string) {},
				events:              makeEvents(3),
				wantOutboxCount:     3,
				wantOnboardingCount: 1,
			},
			{
				name:                "events が 0 件でも、player_onboarding への INSERT は成功する",
				seed:                func(t *testing.T, playerID string) {},
				events:              nil,
				wantOutboxCount:     0,
				wantOnboardingCount: 1,
			},
			{
				name:   "既に完了済みのとき、ErrAlreadyOnboarded になり outbox は積まれない",
				seed:   func(t *testing.T, playerID string) { insertOnboardingRow(t, playerID) },
				events: makeEvents(1),
				// 一意違反で rollback されるため outbox には 1 行も積まれない。onboarding は事前 INSERT 分だけ残る。
				wantErr:             port.ErrAlreadyOnboarded,
				wantOutboxCount:     0,
				wantOnboardingCount: 1,
			},
		}

		for i, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				playerID := newPlayerID(i, 0)
				tt.seed(t, playerID)

				err := repo.MarkComplete(ctx, playerID, tt.events...)
				require.ErrorIs(t, err, tt.wantErr)

				var onboardingCount int
				require.NoError(t, sharedPg.Pool.QueryRow(ctx,
					`SELECT COUNT(*) FROM scenario.player_onboarding WHERE player_id = $1`,
					playerID).Scan(&onboardingCount))
				assert.Equal(t, tt.wantOnboardingCount, onboardingCount)

				var outboxCount int
				require.NoError(t, sharedPg.Pool.QueryRow(ctx,
					`SELECT COUNT(*) FROM scenario.outbox_events`).Scan(&outboxCount))
				assert.Equal(t, tt.wantOutboxCount, outboxCount, "outbox の行数が期待どおり")
			})
		}
	})
}
