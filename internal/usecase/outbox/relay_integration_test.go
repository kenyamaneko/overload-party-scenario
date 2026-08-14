//go:build integration

package outbox_test

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-scenario/internal/adapter/pubsub/pubsubtest"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres/postgrestest"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/outbox"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

const (
	relayIntegrationProjectID = "scenario-outbox-relay-test"
	relayMessageTimeout       = 5 * time.Second
)

var sharedPg *postgrestest.Postgres
var sharedEmulator *pubsubtest.Emulator

// TestMain は Relay の配線 (Postgres outbox → Pub/Sub) を検証するため、
// scenario schema を適用した Postgres コンテナと Pub/Sub emulator をパッケージ共有で起動する。
func TestMain(m *testing.M) {
	ctx := context.Background()

	em, err := pubsubtest.StartEmulator(ctx, relayIntegrationProjectID)
	if err != nil {
		log.Fatalf("start pubsub emulator: %v", err)
	}
	sharedEmulator = em

	code := postgrestest.RunMain(m, &sharedPg,
		postgrestest.WithSchemaFile("db/schema.sql"),
		postgrestest.WithSchema("scenario"),
		postgrestest.WithSearchPath("scenario", "public"),
	)

	if cerr := em.Close(ctx); cerr != nil {
		log.Printf("close pubsub emulator: %v", cerr)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func seedUnpublishedOutboxEvent(t *testing.T, eventID uuid.UUID, eventType string, payload []byte) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.outbox_events (event_id, event_type, payload, created_at, failure_count)
		 VALUES ($1, $2, $3, $4, 0)`,
		eventID, eventType, payload, time.Now(),
	)
	require.NoError(t, err)
}

func findOutboxPublishedAt(t *testing.T, eventID uuid.UUID) *time.Time {
	t.Helper()
	var publishedAt *time.Time
	err := sharedPg.Pool.QueryRow(context.Background(),
		`SELECT published_at FROM scenario.outbox_events WHERE event_id = $1`,
		eventID,
	).Scan(&publishedAt)
	require.NoError(t, err)
	return publishedAt
}

func newRelayTestFixture(t *testing.T) (*outbox.Relay, *pubsubtest.Subscription) {
	t.Helper()
	sharedPg.Truncate(t)
	ctx := context.Background()

	nameSetTopic := sharedEmulator.CreateTopic(t, "TST-RELAY-ONBOARDING-NAME-SET")
	factionSetTopic := sharedEmulator.CreateTopic(t, "TST-RELAY-ONBOARDING-FACTION-SET")
	onboardedTopic := sharedEmulator.CreateTopic(t, "TST-RELAY-PLAYER-ONBOARDED")
	sub := sharedEmulator.Subscribe(t, nameSetTopic)

	pub, err := pubsub.New(ctx, sharedEmulator.ProjectID(), nameSetTopic, factionSetTopic, onboardedTopic)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pub.Close() })

	store := postgres.NewOutboxRepository(sharedPg.Pool)
	relay, err := outbox.New(store, pub, outbox.Config{
		BatchSize:         10,
		FailureThreshold:  3,
		VisibilityTimeout: time.Minute,
	})
	require.NoError(t, err)

	return relay, sub
}

func TestRelay(t *testing.T) {
	t.Run("[配信]outboxイベントの配信", func(t *testing.T) {
		t.Run("outboxに未配信のイベントが登録されているとき、Relayの配信を1回実行すると、そのイベントが実際にPub/Subへメッセージとして届く", func(t *testing.T) {
			relay, sub := newRelayTestFixture(t)
			ctx := context.Background()

			eventID := uuid.New()
			payload := []byte(`{"player_id":"TST-RELAY-PLAYER-0001"}`)
			seedUnpublishedOutboxEvent(t, eventID, apiscenario.EventTypeOnboardingNameSet, payload)

			require.NoError(t, relay.RunOnce(ctx))

			msg, err := sub.WaitForMessage(ctx, relayMessageTimeout)
			require.NoError(t, err)
			assert.JSONEq(t, string(payload), string(msg.Data))
		})

		t.Run("outboxに未配信のイベントが登録されているとき、Relayの配信を1回実行すると、そのイベントが配信済みとして記録される", func(t *testing.T) {
			relay, _ := newRelayTestFixture(t)
			ctx := context.Background()

			eventID := uuid.New()
			payload := []byte(`{"player_id":"TST-RELAY-PLAYER-0002"}`)
			seedUnpublishedOutboxEvent(t, eventID, apiscenario.EventTypeOnboardingNameSet, payload)

			require.NoError(t, relay.RunOnce(ctx))

			assert.NotNil(t, findOutboxPublishedAt(t, eventID))
		})
	})
}
