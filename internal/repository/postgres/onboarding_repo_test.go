//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres"
)

func TestOnboardingRepository_MarkComplete(t *testing.T) {
	repo := postgres.NewOnboardingRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("[オンボーディング]オンボーディング完了の記録", func(t *testing.T) {
		t.Run("対象プレイヤーが未オンボードのとき、オンボード完了記録が作成され、渡したイベントがoutboxに記録される", func(t *testing.T) {
			sharedPg.Truncate(t)
			playerID := uuid.New().String()
			ev := port.OutboxEvent{EventID: uuid.New(), EventType: "TST_PLAYER_ONBOARDED", Payload: []byte(`{"playerId":"p1"}`)}

			require.NoError(t, repo.MarkComplete(ctx, playerID, ev))

			assert.True(t, isOnboarded(t, playerID))
			row, found := findOutboxEvent(t, ev.EventID)
			require.True(t, found)
			assert.Equal(t, ev.EventType, row.EventType)
			assert.JSONEq(t, string(ev.Payload), string(row.Payload))
		})

		t.Run("対象プレイヤーが既にオンボード済みのとき、オンボーディング済みを表すエラーになる", func(t *testing.T) {
			sharedPg.Truncate(t)
			playerID := uuid.New().String()
			require.NoError(t, repo.MarkComplete(ctx, playerID,
				port.OutboxEvent{EventID: uuid.New(), EventType: "TST_PLAYER_ONBOARDED", Payload: []byte(`{}`)}))

			err := repo.MarkComplete(ctx, playerID,
				port.OutboxEvent{EventID: uuid.New(), EventType: "TST_PLAYER_ONBOARDED", Payload: []byte(`{}`)})

			require.ErrorIs(t, err, port.ErrAlreadyOnboarded)
			assert.Equal(t, 1, countOutboxEvents(t))
		})

		t.Run("複数のイベントを渡すと、全て同一トランザクションでoutboxに記録される", func(t *testing.T) {
			sharedPg.Truncate(t)
			playerID := uuid.New().String()
			ev1 := port.OutboxEvent{EventID: uuid.New(), EventType: "TST_NAME_SET", Payload: []byte(`{"k":"name"}`)}
			ev2 := port.OutboxEvent{EventID: uuid.New(), EventType: "TST_FACTION_SET", Payload: []byte(`{"k":"faction"}`)}

			require.NoError(t, repo.MarkComplete(ctx, playerID, ev1, ev2))

			row1, found1 := findOutboxEvent(t, ev1.EventID)
			require.True(t, found1)
			assert.Equal(t, ev1.EventType, row1.EventType)
			assert.JSONEq(t, string(ev1.Payload), string(row1.Payload))

			row2, found2 := findOutboxEvent(t, ev2.EventID)
			require.True(t, found2)
			assert.Equal(t, ev2.EventType, row2.EventType)
			assert.JSONEq(t, string(ev2.Payload), string(row2.Payload))
		})
	})
}

func TestOnboardingRepository_PublishEvents(t *testing.T) {
	repo := postgres.NewOnboardingRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("[オンボーディング]outboxイベントの発行", func(t *testing.T) {
		t.Run("イベントが1件以上あるとき、全てoutboxに記録される", func(t *testing.T) {
			sharedPg.Truncate(t)
			ev1 := port.OutboxEvent{EventID: uuid.New(), EventType: "TST_EVENT_A", Payload: []byte(`{"k":"a"}`)}
			ev2 := port.OutboxEvent{EventID: uuid.New(), EventType: "TST_EVENT_B", Payload: []byte(`{"k":"b"}`)}

			require.NoError(t, repo.PublishEvents(ctx, ev1, ev2))

			row1, found1 := findOutboxEvent(t, ev1.EventID)
			require.True(t, found1)
			assert.Equal(t, ev1.EventType, row1.EventType)
			assert.JSONEq(t, string(ev1.Payload), string(row1.Payload))

			row2, found2 := findOutboxEvent(t, ev2.EventID)
			require.True(t, found2)
			assert.Equal(t, ev2.EventType, row2.EventType)
			assert.JSONEq(t, string(ev2.Payload), string(row2.Payload))
		})

		t.Run("イベントが0件のとき、何も記録されずエラーにならない", func(t *testing.T) {
			sharedPg.Truncate(t)

			require.NoError(t, repo.PublishEvents(ctx))

			assert.Equal(t, 0, countOutboxEvents(t))
		})
	})
}
