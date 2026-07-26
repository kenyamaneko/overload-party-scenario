//go:build integration

package pubsub

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

	"github.com/kenyamaneko/overload-party-scenario/internal/adapter/pubsub/pubsubtest"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

var sharedEmulator *pubsubtest.Emulator

// TestMain は package 共有で emulator を 1 回だけ起動する。
func TestMain(m *testing.M) {
	ctx := context.Background()
	em, err := pubsubtest.StartEmulator(ctx, "scenario-test")
	if err != nil {
		log.Fatalf("start pubsub emulator: %v", err)
	}
	sharedEmulator = em

	code := m.Run()

	if cerr := em.Close(ctx); cerr != nil {
		log.Printf("close emulator: %v", cerr)
	}
	os.Exit(code)
}

// setupPublisher は事前作成した topic に紐付く Publisher を emulator 向けに構築する。
// 物理 topic 名は infra (Terraform) が SSoT であり、test は production と同じ env 由来で解決する。
func setupPublisher(t *testing.T) (*Publisher, publisherTopics) {
	t.Helper()
	envTopics := loadTopicsFromEnv(t)
	topics := publisherTopics{
		onboardingNameSet:    sharedEmulator.CreateTopic(t, envTopics.onboardingNameSet),
		onboardingFactionSet: sharedEmulator.CreateTopic(t, envTopics.onboardingFactionSet),
		playerOnboarded:      sharedEmulator.CreateTopic(t, envTopics.playerOnboarded),
	}

	ctx := context.Background()
	pub, err := New(ctx, sharedEmulator.ProjectID(), topics.onboardingNameSet, topics.onboardingFactionSet, topics.playerOnboarded)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pub.Close() })

	return pub, topics
}

func buildOnboardingNameSetOutbox(t *testing.T, playerID, name string) port.OutboxEvent {
	t.Helper()
	id := uuid.New()
	payload, err := json.Marshal(apiscenario.OnboardingNameSetEvent{
		EventType: apiscenario.EventTypeOnboardingNameSet,
		EventID:   id.String(),
		Timestamp: time.Now().UTC(),
		PlayerID:  playerID,
		Name:      name,
	})
	require.NoError(t, err)
	return port.OutboxEvent{
		EventID:   id,
		EventType: apiscenario.EventTypeOnboardingNameSet,
		Payload:   payload,
	}
}

func buildOnboardingFactionSetOutbox(t *testing.T, playerID, initialFactionID string) port.OutboxEvent {
	t.Helper()
	id := uuid.New()
	payload, err := json.Marshal(apiscenario.OnboardingFactionSetEvent{
		EventType:        apiscenario.EventTypeOnboardingFactionSet,
		EventID:          id.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		InitialFactionID: initialFactionID,
	})
	require.NoError(t, err)
	return port.OutboxEvent{
		EventID:   id,
		EventType: apiscenario.EventTypeOnboardingFactionSet,
		Payload:   payload,
	}
}

func buildPlayerOnboardedOutbox(t *testing.T, playerID, initialFactionID string) port.OutboxEvent {
	t.Helper()
	id := uuid.New()
	payload, err := json.Marshal(apiscenario.PlayerOnboardedEvent{
		EventType:        apiscenario.EventTypePlayerOnboarded,
		EventID:          id.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		InitialFactionID: initialFactionID,
	})
	require.NoError(t, err)
	return port.OutboxEvent{
		EventID:   id,
		EventType: apiscenario.EventTypePlayerOnboarded,
		Payload:   payload,
	}
}

func TestPublishIntegration(t *testing.T) {
	t.Run("Pub/Sub への配信", func(t *testing.T) {
		t.Run("onboarding-name-set を配信すると、購読側に送信内容がそのまま届く", func(t *testing.T) {
			// outbox worker 送出経路の近似。bytes がそのまま subscriber まで届くことを固定する。
			pub, topics := setupPublisher(t)
			sub := sharedEmulator.Subscribe(t, topics.onboardingNameSet)

			ctx := context.Background()
			ev := buildOnboardingNameSetOutbox(t, "player-123", "Kenya")
			require.NoError(t, pub.Publish(ctx, ev.EventType, ev.Payload))

			msg, err := sub.WaitForMessage(ctx, 5*time.Second)
			require.NoError(t, err)

			var decoded apiscenario.OnboardingNameSetEvent
			require.NoError(t, json.Unmarshal(msg.Data, &decoded))

			assert.Equal(t, apiscenario.EventTypeOnboardingNameSet, decoded.EventType)
			assert.Equal(t, ev.EventID.String(), decoded.EventID, "payload の event_id は outbox 行の PK と一致する")
			assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
			assert.Equal(t, "player-123", decoded.PlayerID)
			assert.Equal(t, "Kenya", decoded.Name)
		})

		t.Run("onboarding-faction-set を配信すると、送信内容が保たれる", func(t *testing.T) {
			pub, topics := setupPublisher(t)
			sub := sharedEmulator.Subscribe(t, topics.onboardingFactionSet)

			ctx := context.Background()
			ev := buildOnboardingFactionSetOutbox(t, "player-456", "SHE")
			require.NoError(t, pub.Publish(ctx, ev.EventType, ev.Payload))

			msg, err := sub.WaitForMessage(ctx, 5*time.Second)
			require.NoError(t, err)

			var decoded apiscenario.OnboardingFactionSetEvent
			require.NoError(t, json.Unmarshal(msg.Data, &decoded))

			assert.Equal(t, apiscenario.EventTypeOnboardingFactionSet, decoded.EventType)
			assert.Equal(t, ev.EventID.String(), decoded.EventID)
			assert.Equal(t, "player-456", decoded.PlayerID)
			assert.Equal(t, "SHE", decoded.InitialFactionID)
		})

		t.Run("player-onboarded を配信すると、送信内容が保たれる", func(t *testing.T) {
			pub, topics := setupPublisher(t)
			sub := sharedEmulator.Subscribe(t, topics.playerOnboarded)

			ctx := context.Background()
			ev := buildPlayerOnboardedOutbox(t, "player-789", "Tenki")
			require.NoError(t, pub.Publish(ctx, ev.EventType, ev.Payload))

			msg, err := sub.WaitForMessage(ctx, 5*time.Second)
			require.NoError(t, err)

			var decoded apiscenario.PlayerOnboardedEvent
			require.NoError(t, json.Unmarshal(msg.Data, &decoded))

			assert.Equal(t, apiscenario.EventTypePlayerOnboarded, decoded.EventType)
			assert.Equal(t, ev.EventID.String(), decoded.EventID)
			assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
			assert.Equal(t, "player-789", decoded.PlayerID)
			assert.Equal(t, "Tenki", decoded.InitialFactionID)
		})

		t.Run("未登録のイベント種別を配信すると、エラーになる", func(t *testing.T) {
			// outbox 行の eventType 設定ミスを worker の failure_count に積ませるため、
			// 未登録 eventType は Pub/Sub SDK 到達前に adapter 側で弾く。
			pub, _ := setupPublisher(t)

			err := pub.Publish(context.Background(), "not-registered-event-type", []byte(`{}`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown event type")
		})

		t.Run("配信しなければ、購読側はタイムアウトする", func(t *testing.T) {
			// 正例テストの偽陽性除け。
			_, topics := setupPublisher(t)
			sub := sharedEmulator.Subscribe(t, topics.playerOnboarded)

			_, err := sub.WaitForMessage(context.Background(), 500*time.Millisecond)
			assert.ErrorIs(t, err, pubsubtest.ErrTimeout)
		})
	})
}
