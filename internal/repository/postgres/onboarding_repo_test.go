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

	tests := []struct {
		name            string
		events          []port.OutboxEvent
		wantOutboxCount int
	}{
		{
			name:            "events 複数: 単一 tx で全行が outbox に積まれる",
			events:          makeEvents(2),
			wantOutboxCount: 2,
		},
		{
			name:            "events 1 件: 単独でも outbox に 1 行積む",
			events:          makeEvents(1),
			wantOutboxCount: 1,
		},
		{
			name:            "events 0 件: no-op で outbox に何も積まない",
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

	tests := []struct {
		name             string
		preInsert        bool // 先に同 playerID を INSERT して二度目シナリオを作るか
		events           []port.OutboxEvent
		wantErrIs        error
		wantOutboxCount  int // 成功時に outbox に積まれているべき行数
		wantOnboardedRow bool
	}{
		{
			name:             "正常系: events が複数でも atomic に outbox へ全て INSERT される",
			preInsert:        false,
			events:           makeEvents(3),
			wantOutboxCount:  3,
			wantOnboardedRow: true,
		},
		{
			name:             "events 0 件でも player_onboarding への INSERT は成功する",
			preInsert:        false,
			events:           nil,
			wantOutboxCount:  0,
			wantOnboardedRow: true,
		},
		{
			name:      "二度目の MarkComplete は ErrAlreadyOnboarded を返す",
			preInsert: true,
			events:    makeEvents(1),
			wantErrIs: port.ErrAlreadyOnboarded,
			// 一意違反で rollback されるため outbox には 1 行も積まれない。
			wantOutboxCount:  0,
			wantOnboardedRow: true, // 事前 INSERT 分だけ残る
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sharedPg.Truncate(t)
			playerID := newPlayerID(i, 0)

			// 二度目シナリオでは事前に同 playerID の行を作っておく。
			setup := map[bool]func(){
				true: func() { insertOnboardingRow(t, playerID) },
				false: func() {},
			}
			setup[tt.preInsert]()

			err := repo.MarkComplete(ctx, playerID, tt.events...)

			errCheck := map[bool]func(){
				true: func() {
					assert.ErrorIs(t, err, tt.wantErrIs)
				},
				false: func() {
					require.NoError(t, err)
				},
			}
			errCheck[tt.wantErrIs != nil]()

			// onboarding 行の存在確認
			var onboardingCount int
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM scenario.player_onboarding WHERE player_id = $1`,
				playerID).Scan(&onboardingCount))
			wantOnboardingCount := map[bool]int{true: 1, false: 0}[tt.wantOnboardedRow]
			assert.Equal(t, wantOnboardingCount, onboardingCount)

			// outbox 行の件数確認 (atomic INSERT の保証)
			var outboxCount int
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM scenario.outbox_events`).Scan(&outboxCount))
			assert.Equal(t, tt.wantOutboxCount, outboxCount, "outbox の行数が期待どおり")
		})
	}
}
