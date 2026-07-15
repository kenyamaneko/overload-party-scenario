//go:build integration

package outbox_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	scenariopubsub "github.com/kenyamaneko/overload-party-scenario/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-scenario/internal/adapter/pubsub/pubsubtest"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres/postgrestest"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/outbox"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

var (
	sharedPg       *postgrestest.Postgres
	sharedEmulator *pubsubtest.Emulator
)

// TestMain は package 共有で Postgres と Pub/Sub emulator を 1 回だけ起動する。
func TestMain(m *testing.M) {
	ctx := context.Background()

	pg, err := postgrestest.Start(ctx,
		postgrestest.WithSchemaFile("db/schema.sql"),
		postgrestest.WithSchemaFile("internal/repository/postgres/testdata/account_stub.sql"),
		postgrestest.WithSchema("scenario"),
		postgrestest.WithSearchPath("scenario", "public"),
	)
	if err != nil {
		log.Fatalf("postgrestest.Start: %v", err)
	}
	sharedPg = pg

	em, err := pubsubtest.StartEmulator(ctx, "scenario-test")
	if err != nil {
		_ = pg.Close(ctx)
		log.Fatalf("start pubsub emulator: %v", err)
	}
	sharedEmulator = em

	code := m.Run()

	if cerr := em.Close(ctx); cerr != nil {
		log.Printf("close emulator: %v", cerr)
	}
	if cerr := pg.Close(ctx); cerr != nil {
		log.Printf("close postgres: %v", cerr)
	}
	os.Exit(code)
}

// setupRelay は既定設定で setupRelayWithConfig を呼ぶ。
func setupRelay(t *testing.T) (*outbox.Relay, *pubsubtest.Subscription, string) {
	t.Helper()
	return setupRelayWithConfig(t, outbox.Config{
		BatchSize:         10,
		FailureThreshold:  5,
		VisibilityTimeout: 30 * time.Second,
	})
}

// setupRelayWithConfig は emulator に scenario の全 topic を作り、Publisher と OutboxRepository を組み上げて
// 指定 Config で Relay を返す。物理 topic 名は infra (Terraform) が SSoT のため、本テストではリテラルで宣言する。
// 戻り値の topic は player_onboarded 用 (EventTypePlayerOnboarded を扱う test ケースが受信側で参照する)。
func setupRelayWithConfig(t *testing.T, cfg outbox.Config) (*outbox.Relay, *pubsubtest.Subscription, string) {
	t.Helper()
	sharedPg.Truncate(t)

	onboardingNameSetTopic := sharedEmulator.CreateTopic(t, "onboarding-name-set")
	onboardingFactionSetTopic := sharedEmulator.CreateTopic(t, "onboarding-faction-set")
	playerOnboardedTopic := sharedEmulator.CreateTopic(t, "player-onboarded")
	sub := sharedEmulator.Subscribe(t, playerOnboardedTopic)

	ctx := context.Background()
	pub, err := scenariopubsub.New(ctx, sharedEmulator.ProjectID(), onboardingNameSetTopic, onboardingFactionSetTopic, playerOnboardedTopic)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pub.Close() })

	store := postgres.NewOutboxRepository(sharedPg.Pool)
	relay, err := outbox.New(store, pub, cfg)
	require.NoError(t, err)
	return relay, sub, playerOnboardedTopic
}

// backdateLastAttempted は last_attempted_at を過去方向に移動させ、別 worker が既に
// 試行中 (visibility timeout 内) もしくは試行後 (timeout 超過) の状態を作る。
func backdateLastAttempted(t *testing.T, id uuid.UUID, ago time.Duration) {
	t.Helper()
	interval := fmt.Sprintf("%d milliseconds", ago.Milliseconds())
	_, err := sharedPg.Pool.Exec(context.Background(),
		`UPDATE scenario.outbox_events SET last_attempted_at = now() - ($2::text)::interval WHERE event_id = $1`,
		id, interval)
	require.NoError(t, err)
}

// setFailureCountDirectly は seed 後に failure_count を直接指定値に更新する。
func setFailureCountDirectly(t *testing.T, id uuid.UUID, count int) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`UPDATE scenario.outbox_events SET failure_count = $2 WHERE event_id = $1`, id, count)
	require.NoError(t, err)
}

// insertOutboxRow は scenario.outbox_events に 1 行 INSERT して event_id を返す。
func insertOutboxRow(t *testing.T, eventType string, payload []byte) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.outbox_events (event_id, event_type, payload, failure_count)
		 VALUES ($1, $2, $3, 0)`,
		id, eventType, payload)
	require.NoError(t, err)
	return id
}

// fetchOutboxState は published_at / failure_count / last_error を返す。
func fetchOutboxState(t *testing.T, id uuid.UUID) (publishedAt *time.Time, failureCount int, lastError *string) {
	t.Helper()
	row := sharedPg.Pool.QueryRow(context.Background(),
		`SELECT published_at, failure_count, last_error
		   FROM scenario.outbox_events
		  WHERE event_id = $1`, id)
	require.NoError(t, row.Scan(&publishedAt, &failureCount, &lastError))
	return
}

func TestRunOnceIntegration(t *testing.T) {
	t.Run("Relay.RunOnce の Pub/Sub 連携", func(t *testing.T) {
		t.Run("未配信行があるとき、publish して published_at をマークする", func(t *testing.T) {
			relay, sub, _ := setupRelay(t)

			payload := []byte(`{"event_type":"player_onboarded","player_id":"p-1","initial_faction_id":"Tenki"}`)
			id := insertOutboxRow(t, apiscenario.EventTypePlayerOnboarded, payload)

			require.NoError(t, relay.RunOnce(context.Background()))

			msg, err := sub.WaitForMessage(context.Background(), 5*time.Second)
			require.NoError(t, err)
			assert.JSONEq(t, string(payload), string(msg.Data))

			publishedAt, failureCount, lastError := fetchOutboxState(t, id)
			require.NotNil(t, publishedAt, "published_at must be set after successful publish")
			assert.WithinDuration(t, time.Now(), *publishedAt, 5*time.Second)
			assert.Equal(t, 0, failureCount)
			assert.Nil(t, lastError)
		})

		t.Run("typed payload を publish すると、subscriber 側で decode できる", func(t *testing.T) {
			relay, sub, _ := setupRelay(t)

			id := uuid.New()
			payload, err := json.Marshal(apiscenario.PlayerOnboardedEvent{
				EventType:        apiscenario.EventTypePlayerOnboarded,
				EventID:          id.String(),
				Timestamp:        time.Now().UTC(),
				PlayerID:         "player-xyz",
				InitialFactionID: "SHE",
			})
			require.NoError(t, err)
			_, err = sharedPg.Pool.Exec(context.Background(),
				`INSERT INTO scenario.outbox_events (event_id, event_type, payload, failure_count)
				 VALUES ($1, $2, $3, 0)`,
				id, apiscenario.EventTypePlayerOnboarded, payload)
			require.NoError(t, err)

			require.NoError(t, relay.RunOnce(context.Background()))

			msg, err := sub.WaitForMessage(context.Background(), 5*time.Second)
			require.NoError(t, err)

			var decoded apiscenario.PlayerOnboardedEvent
			require.NoError(t, json.Unmarshal(msg.Data, &decoded))
			assert.Equal(t, apiscenario.EventTypePlayerOnboarded, decoded.EventType)
			assert.Equal(t, id.String(), decoded.EventID)
			assert.Equal(t, "player-xyz", decoded.PlayerID)
			assert.Equal(t, "SHE", decoded.InitialFactionID)
		})

		t.Run("未登録の event type のとき、publish されず RecordFailure が記録される", func(t *testing.T) {
			relay, sub, _ := setupRelay(t)

			const wrongEventType = "wrong-event-type"
			id := insertOutboxRow(t, wrongEventType, []byte(`{"k":"v"}`))

			require.NoError(t, relay.RunOnce(context.Background()))

			_, err := sub.WaitForMessage(context.Background(), 300*time.Millisecond)
			assert.ErrorIs(t, err, pubsubtest.ErrTimeout)

			publishedAt, failureCount, lastError := fetchOutboxState(t, id)
			assert.Nil(t, publishedAt, "published_at must remain NULL on failure")
			assert.Equal(t, 1, failureCount)
			require.NotNil(t, lastError)
			assert.Contains(t, *lastError, "unknown event type")
		})

		t.Run("既配信行のとき、再度 publish されない", func(t *testing.T) {
			relay, sub, _ := setupRelay(t)

			id := insertOutboxRow(t, apiscenario.EventTypePlayerOnboarded, []byte(`{"k":"v"}`))
			_, err := sharedPg.Pool.Exec(context.Background(),
				`UPDATE scenario.outbox_events SET published_at = now() WHERE event_id = $1`, id)
			require.NoError(t, err)

			require.NoError(t, relay.RunOnce(context.Background()))

			_, err = sub.WaitForMessage(context.Background(), 300*time.Millisecond)
			assert.ErrorIs(t, err, pubsubtest.ErrTimeout, "既配信行は再度 publish されない")
		})

		t.Run("バッチサイズを超える未配信行があるとき、バッチサイズ件だけ publish され残りは未配信のまま残る", func(t *testing.T) {
			relay, sub, _ := setupRelayWithConfig(t, outbox.Config{BatchSize: 1, FailureThreshold: 5, VisibilityTimeout: 30 * time.Second})

			id1 := insertOutboxRow(t, apiscenario.EventTypePlayerOnboarded, []byte(`{"k":"1"}`))
			id2 := insertOutboxRow(t, apiscenario.EventTypePlayerOnboarded, []byte(`{"k":"2"}`))

			require.NoError(t, relay.RunOnce(context.Background()))

			msgs, err := sub.WaitForN(context.Background(), 1, 2*time.Second)
			require.NoError(t, err)
			assert.Len(t, msgs, 1)
			_, err = sub.WaitForMessage(context.Background(), 300*time.Millisecond)
			assert.ErrorIs(t, err, pubsubtest.ErrTimeout, "バッチサイズが 1 なので2件目は今回の RunOnce で publish されない")

			publishedAt1, _, _ := fetchOutboxState(t, id1)
			publishedAt2, _, _ := fetchOutboxState(t, id2)
			publishedCount := 0
			for _, p := range []*time.Time{publishedAt1, publishedAt2} {
				if p != nil {
					publishedCount++
				}
			}
			assert.Equal(t, 1, publishedCount, "バッチサイズが 1 なのでちょうど1件だけ published_at が立つ")
		})

		t.Run("可視性タイムアウト以内で試行中の行はスキップされ、超過した行は再び publish される", func(t *testing.T) {
			relay, sub, _ := setupRelayWithConfig(t, outbox.Config{BatchSize: 10, FailureThreshold: 5, VisibilityTimeout: 200 * time.Millisecond})

			inFlightID := insertOutboxRow(t, apiscenario.EventTypePlayerOnboarded, []byte(`{"k":"in-flight"}`))
			backdateLastAttempted(t, inFlightID, 50*time.Millisecond)

			recoveredID := insertOutboxRow(t, apiscenario.EventTypePlayerOnboarded, []byte(`{"k":"recovered"}`))
			backdateLastAttempted(t, recoveredID, 500*time.Millisecond)

			require.NoError(t, relay.RunOnce(context.Background()))

			msg, err := sub.WaitForMessage(context.Background(), 2*time.Second)
			require.NoError(t, err)
			assert.JSONEq(t, `{"k":"recovered"}`, string(msg.Data))
			_, err = sub.WaitForMessage(context.Background(), 300*time.Millisecond)
			assert.ErrorIs(t, err, pubsubtest.ErrTimeout, "試行中の行は今回の RunOnce では publish されない")

			publishedAtInFlight, _, _ := fetchOutboxState(t, inFlightID)
			assert.Nil(t, publishedAtInFlight, "可視性タイムアウト以内の行はスキップされる")

			publishedAtRecovered, _, _ := fetchOutboxState(t, recoveredID)
			assert.NotNil(t, publishedAtRecovered, "可視性タイムアウト超過の行は再び publish される")
		})

		t.Run("失敗回数の上限に到達した行は処理対象に含まれず、publish されない", func(t *testing.T) {
			relay, sub, _ := setupRelayWithConfig(t, outbox.Config{BatchSize: 10, FailureThreshold: 2, VisibilityTimeout: 30 * time.Second})

			exhaustedID := insertOutboxRow(t, apiscenario.EventTypePlayerOnboarded, []byte(`{"k":"exhausted"}`))
			setFailureCountDirectly(t, exhaustedID, 2)

			require.NoError(t, relay.RunOnce(context.Background()))

			_, err := sub.WaitForMessage(context.Background(), 300*time.Millisecond)
			assert.ErrorIs(t, err, pubsubtest.ErrTimeout, "失敗回数の上限到達行は処理対象に含まれず publish されない")

			publishedAt, failureCount, _ := fetchOutboxState(t, exhaustedID)
			assert.Nil(t, publishedAt)
			assert.Equal(t, 2, failureCount, "処理対象に含まれないので failure_count は変化しない")
		})
	})
}
