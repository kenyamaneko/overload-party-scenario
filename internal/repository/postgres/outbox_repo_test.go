//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres"
)

const (
	outboxDefaultVisibility = 30 * time.Second
	outboxDefaultThreshold  = 3
)

// seedClaimCandidate は failure_count・last_attempted_at (nil なら未試行) を指定した outbox 行を 1 件 seed する。
func seedClaimCandidate(t *testing.T, failureCount int, attemptedAgo *time.Duration) uuid.UUID {
	t.Helper()
	id := uuid.New()
	var lastAttemptedAt *time.Time
	if attemptedAgo != nil {
		at := time.Now().Add(-*attemptedAgo)
		lastAttemptedAt = &at
	}
	seedOutboxEvent(t, id, "TST_EVENT_TYPE", []byte(`{"k":"v"}`), time.Now(), lastAttemptedAt, failureCount, nil)
	return id
}

func containsEventID(claimed []port.ClaimedOutboxEvent, id uuid.UUID) bool {
	for _, ev := range claimed {
		if ev.EventID == id {
			return true
		}
	}
	return false
}

func TestOutboxRepository_ClaimUnpublished(t *testing.T) {
	repo := postgres.NewOutboxRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("[配信]未配信行の取得", func(t *testing.T) {
		withinVisibility := 5 * time.Second
		beyondVisibility := 60 * time.Second

		tests := []struct {
			name         string
			failureCount int
			attemptedAgo *time.Duration
			wantClaimed  bool
		}{
			{
				name:         "直近visibilityTimeout以内に試行された行は、claim対象に含まれない",
				failureCount: 0,
				attemptedAgo: &withinVisibility,
				wantClaimed:  false,
			},
			{
				name:         "visibilityTimeoutを超えて試行された行は、claim対象に含まれる",
				failureCount: 0,
				attemptedAgo: &beyondVisibility,
				wantClaimed:  true,
			},
			{
				name:         "未試行の行は、claim対象に含まれる",
				failureCount: 0,
				attemptedAgo: nil,
				wantClaimed:  true,
			},
			{
				name:         "failure_countがfailureThreshold未満の行はclaim対象に含まれる",
				failureCount: 1,
				attemptedAgo: &beyondVisibility,
				wantClaimed:  true,
			},
			{
				name:         "failure_countがfailureThresholdと等しい行はclaim対象に含まれない",
				failureCount: outboxDefaultThreshold,
				attemptedAgo: nil,
				wantClaimed:  false,
			},
			{
				name:         "failure_countがfailureThresholdの直下(threshold-1)の行はclaim対象に含まれる",
				failureCount: outboxDefaultThreshold - 1,
				attemptedAgo: nil,
				wantClaimed:  true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				id := seedClaimCandidate(t, tt.failureCount, tt.attemptedAgo)

				claimed, err := repo.ClaimUnpublished(ctx, 10, outboxDefaultVisibility, outboxDefaultThreshold)
				require.NoError(t, err)

				assert.Equal(t, tt.wantClaimed, containsEventID(claimed, id))
			})
		}

		t.Run("未配信の行があるとき、claimされ、その行のイベント種別・payload・失敗回数が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			id := uuid.New()
			seedOutboxEvent(t, id, "TST_EVENT_TYPE_ECHO", []byte(`{"k":"echo"}`), time.Now(), nil, 2, nil)

			claimed, err := repo.ClaimUnpublished(ctx, 10, outboxDefaultVisibility, outboxDefaultThreshold)
			require.NoError(t, err)
			require.Len(t, claimed, 1)
			assert.Equal(t, id, claimed[0].EventID)
			assert.Equal(t, "TST_EVENT_TYPE_ECHO", claimed[0].EventType)
			assert.JSONEq(t, `{"k":"echo"}`, string(claimed[0].Payload))
			assert.Equal(t, 2, claimed[0].FailureCount)
		})

		t.Run("未配信の行がlimit件を超えて存在するとき、claimされるのはlimit件までで、残りは未配信のまま残る", func(t *testing.T) {
			sharedPg.Truncate(t)
			const totalSeeded = 3
			const limit = 2
			for i := 0; i < totalSeeded; i++ {
				seedClaimCandidate(t, 0, nil)
			}

			claimed, err := repo.ClaimUnpublished(ctx, limit, outboxDefaultVisibility, outboxDefaultThreshold)
			require.NoError(t, err)
			assert.Len(t, claimed, limit)

			var remaining int
			require.NoError(t, sharedPg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM scenario.outbox_events
				  WHERE published_at IS NULL AND last_attempted_at IS NULL`,
			).Scan(&remaining))
			assert.Equal(t, totalSeeded-limit, remaining)
		})

		t.Run("claim対象の行が複数あるとき、作成日時が古い順に返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			now := time.Now()
			oldest := uuid.New()
			middle := uuid.New()
			newest := uuid.New()
			seedOutboxEvent(t, newest, "TST_EVENT_TYPE", []byte(`{"k":"3"}`), now.Add(-1*time.Second), nil, 0, nil)
			seedOutboxEvent(t, oldest, "TST_EVENT_TYPE", []byte(`{"k":"1"}`), now.Add(-3*time.Second), nil, 0, nil)
			seedOutboxEvent(t, middle, "TST_EVENT_TYPE", []byte(`{"k":"2"}`), now.Add(-2*time.Second), nil, 0, nil)

			claimed, err := repo.ClaimUnpublished(ctx, 10, outboxDefaultVisibility, outboxDefaultThreshold)
			require.NoError(t, err)
			require.Len(t, claimed, 3)
			got := []uuid.UUID{claimed[0].EventID, claimed[1].EventID, claimed[2].EventID}
			assert.Equal(t, []uuid.UUID{oldest, middle, newest}, got)
		})

		t.Run("claim対象の行が無いとき、空の結果が返る", func(t *testing.T) {
			sharedPg.Truncate(t)

			claimed, err := repo.ClaimUnpublished(ctx, 10, outboxDefaultVisibility, outboxDefaultThreshold)
			require.NoError(t, err)
			assert.Empty(t, claimed)
		})

		t.Run("claimされた行は試行日時が更新され、以降visibilityTimeout以内は再claim対象から外れる", func(t *testing.T) {
			sharedPg.Truncate(t)
			id := seedClaimCandidate(t, 0, nil)

			first, err := repo.ClaimUnpublished(ctx, 10, outboxDefaultVisibility, outboxDefaultThreshold)
			require.NoError(t, err)
			assert.True(t, containsEventID(first, id))

			row, found := findOutboxEvent(t, id)
			require.True(t, found)
			assert.NotNil(t, row.LastAttemptedAt)

			second, err := repo.ClaimUnpublished(ctx, 10, outboxDefaultVisibility, outboxDefaultThreshold)
			require.NoError(t, err)
			assert.False(t, containsEventID(second, id))
		})
	})
}

func TestOutboxRepository_MarkPublished(t *testing.T) {
	repo := postgres.NewOutboxRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("[配信]配信済みの記録", func(t *testing.T) {
		t.Run("対象行が存在するとき、MarkPublishedを呼ぶとpublish済みとして記録され、以降claim対象から外れる", func(t *testing.T) {
			sharedPg.Truncate(t)
			id := seedClaimCandidate(t, 0, nil)

			require.NoError(t, repo.MarkPublished(ctx, id))

			row, found := findOutboxEvent(t, id)
			require.True(t, found)
			assert.NotNil(t, row.PublishedAt)

			claimed, err := repo.ClaimUnpublished(ctx, 10, outboxDefaultVisibility, outboxDefaultThreshold)
			require.NoError(t, err)
			assert.False(t, containsEventID(claimed, id))
		})

		t.Run("対象行にエラーが記録されていた場合、MarkPublished呼び出し後はエラー記録がクリアされる", func(t *testing.T) {
			sharedPg.Truncate(t)
			id := seedClaimCandidate(t, 0, nil)
			require.NoError(t, repo.RecordFailure(ctx, id, "boom"))

			require.NoError(t, repo.MarkPublished(ctx, id))

			row, found := findOutboxEvent(t, id)
			require.True(t, found)
			assert.Nil(t, row.LastError)
		})
	})
}

func TestOutboxRepository_RecordFailure(t *testing.T) {
	repo := postgres.NewOutboxRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("[配信]失敗の記録", func(t *testing.T) {
		t.Run("対象行が存在するとき、RecordFailureを呼ぶと失敗回数が1増加する", func(t *testing.T) {
			sharedPg.Truncate(t)
			id := seedClaimCandidate(t, 0, nil)

			require.NoError(t, repo.RecordFailure(ctx, id, "boom"))

			row, found := findOutboxEvent(t, id)
			require.True(t, found)
			assert.Equal(t, 1, row.FailureCount)
		})

		t.Run("RecordFailureを呼ぶと、対象行の直近エラー内容が指定したメッセージに更新される", func(t *testing.T) {
			sharedPg.Truncate(t)
			id := seedClaimCandidate(t, 0, nil)

			require.NoError(t, repo.RecordFailure(ctx, id, "pubsub down"))

			row, found := findOutboxEvent(t, id)
			require.True(t, found)
			require.NotNil(t, row.LastError)
			assert.Equal(t, "pubsub down", *row.LastError)
		})

		t.Run("RecordFailureを繰り返すと、失敗回数が呼び出し回数分累積する", func(t *testing.T) {
			sharedPg.Truncate(t)
			id := seedClaimCandidate(t, 0, nil)

			require.NoError(t, repo.RecordFailure(ctx, id, "err-1"))
			require.NoError(t, repo.RecordFailure(ctx, id, "err-2"))
			require.NoError(t, repo.RecordFailure(ctx, id, "err-3"))

			row, found := findOutboxEvent(t, id)
			require.True(t, found)
			assert.Equal(t, 3, row.FailureCount)
		})
	})
}
