//go:build integration

// このファイルは outbox の claim → publish → mark/fail を「実 Postgres + 実 Pub/Sub
// emulator」に対して end-to-end で固定する。unit test (relay_test.go) は
// fakeStore + fakeRawPublisher で order/分岐を検証するのに対し、ここでは
// scenario.outbox_events の SQL 振る舞いと Pub/Sub SDK 経路の組み合わせ動作を
// 担保する (型の組み立てミスや SQL の更新条件ずれが unit test から漏れていないか)。
package outbox_test

import (
	"context"
	"encoding/json"
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
	"github.com/kenyamaneko/overload-party-scenario/internal/service/outbox"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

var (
	sharedPg       *postgrestest.Postgres
	sharedEmulator *pubsubtest.Emulator
)

// TestMain は package 内の integration test 共有で Postgres と Pub/Sub emulator を
// 1 回だけ起動する。container 起動コストは高いため per-test ではなく package scope で
// 償却する。テスト間の状態リセットは sharedPg.Truncate と topic UUID suffix で行う。
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

// setupRelay は emulator に topic を 1 本作り、その topic を Publisher に紐づけて
// OutboxRepository / outbox.Relay (service) を組み上げる。戻り値は service と
// subscription、および topic 名 (subscriber 待受確認用)。
func setupRelay(t *testing.T) (*outbox.Relay, *pubsubtest.Subscription, string) {
	t.Helper()
	sharedPg.Truncate(t)

	topic := sharedEmulator.CreateTopic(t, apiscenario.TopicPlayerOnboarded)
	sub := sharedEmulator.Subscribe(t, topic)

	ctx := context.Background()
	pub, err := scenariopubsub.New(ctx, sharedEmulator.ProjectID(), topic)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pub.Close() })

	store := postgres.NewOutboxRepository(sharedPg.Pool)
	relay, err := outbox.New(store, pub, outbox.Config{
		BatchSize:         10,
		FailureThreshold:  5,
		VisibilityTimeout: 30 * time.Second,
	})
	require.NoError(t, err)
	return relay, sub, topic
}

// insertOutboxRow は scenario.outbox_events に 1 行直接 INSERT し、event_id を返す。
// 書き込み API は package 外に公開していないため raw SQL で seed する
// (outbox_repo_test.go と同じ構造)。
func insertOutboxRow(t *testing.T, eventType string, payload []byte) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.outbox_events (event_id, event_type, payload)
		 VALUES ($1, $2, $3)`,
		id, eventType, payload)
	require.NoError(t, err)
	return id
}

// fetchOutboxState は published_at と failure_count を返す (NULL は zero 値で復元)。
// claim → publish 後の行状態を assertion で固定するために使う。
func fetchOutboxState(t *testing.T, id uuid.UUID) (publishedAt *time.Time, failureCount int, lastError *string) {
	t.Helper()
	row := sharedPg.Pool.QueryRow(context.Background(),
		`SELECT published_at, failure_count, last_error
		   FROM scenario.outbox_events
		  WHERE event_id = $1`, id)
	require.NoError(t, row.Scan(&publishedAt, &failureCount, &lastError))
	return
}

// 未配信行が claim → publish → MarkPublished の順序で処理され、subscriber まで
// payload がそのまま到達することを固定する。outbox の at-least-once 契約上、
// payload の中身は worker からは不透明 (service 層で構築済み) であり、bytes
// として透過することを保証する。
func TestIntegration_RunOnce_PublishesAndMarks(t *testing.T) {
	relay, sub, _ := setupRelay(t)

	payload := []byte(`{"event_type":"player_onboarded","player_id":"p-1","display_name":"DN","initial_faction_id":"Tenki"}`)
	id := insertOutboxRow(t, apiscenario.EventTypePlayerOnboarded, payload)

	require.NoError(t, relay.RunOnce(context.Background()))

	// emulator subscriber 経由で到達確認 (bytes は seed と完全一致)。
	msg, err := sub.WaitForMessage(context.Background(), 5*time.Second)
	require.NoError(t, err)
	assert.JSONEq(t, string(payload), string(msg.Data))

	// row は published_at が立ち、failure_count / last_error は触られない。
	publishedAt, failureCount, lastError := fetchOutboxState(t, id)
	require.NotNil(t, publishedAt, "published_at must be set after successful publish")
	assert.WithinDuration(t, time.Now(), *publishedAt, 5*time.Second)
	assert.Equal(t, 0, failureCount)
	assert.Nil(t, lastError)
}

// service 層が組み立てる shape の payload (apiscenario.PlayerOnboardedEvent JSON) を
// outbox に積んで Publisher で送出したときに、subscriber 側で round-trip できることを
// 固定する。relay_test.go の fakeRawPublisher を本物に差し替えた経路の検証。
func TestIntegration_RunOnce_DeliversTypedPayload(t *testing.T) {
	relay, sub, _ := setupRelay(t)

	id := uuid.New()
	payload, err := json.Marshal(apiscenario.PlayerOnboardedEvent{
		EventType:        apiscenario.EventTypePlayerOnboarded,
		EventID:          id.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         "player-xyz",
		DisplayName:      "Display Name",
		InitialFactionID: "SHE",
	})
	require.NoError(t, err)
	_, err = sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.outbox_events (event_id, event_type, payload)
		 VALUES ($1, $2, $3)`,
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
	assert.Equal(t, "Display Name", decoded.DisplayName)
	assert.Equal(t, "SHE", decoded.InitialFactionID)
}

// adapter で未登録の eventType (= outbox 行の eventType 設定ミス) に対しては
// publish がエラーになり、行は MarkPublished されず RecordFailure (failure_count++ /
// last_error 記録) で終わる契約を固定する。閾値超過は visibility timeout 経過後の
// 再試行で拾うため、RunOnce 自体はエラーにならない。
func TestIntegration_RunOnce_UnknownEventType_RecordsFailure(t *testing.T) {
	relay, sub, _ := setupRelay(t)

	const wrongEventType = "wrong-event-type"
	id := insertOutboxRow(t, wrongEventType, []byte(`{"k":"v"}`))

	require.NoError(t, relay.RunOnce(context.Background()))

	// subscriber には何も届かない (publish 自体が adapter で弾かれている)。
	_, err := sub.WaitForMessage(context.Background(), 300*time.Millisecond)
	assert.ErrorIs(t, err, pubsubtest.ErrTimeout)

	publishedAt, failureCount, lastError := fetchOutboxState(t, id)
	assert.Nil(t, publishedAt, "published_at must remain NULL on failure")
	assert.Equal(t, 1, failureCount)
	require.NotNil(t, lastError)
	assert.Contains(t, *lastError, "unknown event type")
}

// 既配信行 (published_at セット済み) は claim 対象から除外され、publish は走らない。
// outbox_repo_test.go でも個別に検証しているが、ここでは publish 経路まで含めた
// 「重複配信されない」契約として固定する。
func TestIntegration_RunOnce_AlreadyPublished_NoOp(t *testing.T) {
	relay, sub, _ := setupRelay(t)

	id := insertOutboxRow(t, apiscenario.EventTypePlayerOnboarded, []byte(`{"k":"v"}`))
	_, err := sharedPg.Pool.Exec(context.Background(),
		`UPDATE scenario.outbox_events SET published_at = now() WHERE event_id = $1`, id)
	require.NoError(t, err)

	require.NoError(t, relay.RunOnce(context.Background()))

	_, err = sub.WaitForMessage(context.Background(), 300*time.Millisecond)
	assert.ErrorIs(t, err, pubsubtest.ErrTimeout, "既配信行は再度 publish されない")
}
