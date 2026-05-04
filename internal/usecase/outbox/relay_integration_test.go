//go:build integration

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

// setupRelay は emulator に topic を 1 本作り、Publisher と OutboxRepository を組み上げて Relay を返す。
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

func TestIntegration_RunOnce_PublishesAndMarks(t *testing.T) {
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
}

func TestIntegration_RunOnce_DeliversTypedPayload(t *testing.T) {
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
}

func TestIntegration_RunOnce_UnknownEventType_RecordsFailure(t *testing.T) {
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
}

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
