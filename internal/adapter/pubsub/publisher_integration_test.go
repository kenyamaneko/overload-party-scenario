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

// TestMain は package 共有で emulator を 1 回だけ起動する (起動コストを
// package scope で償却)。テスト間の分離は topic / subscription の UUID suffix。
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
func setupPublisher(t *testing.T) (*Publisher, string) {
	t.Helper()
	topic := sharedEmulator.CreateTopic(t, apiscenario.TopicPlayerOnboarded)

	ctx := context.Background()
	pub, err := New(ctx, sharedEmulator.ProjectID(), topic)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pub.Close() })

	return pub, topic
}

// buildPlayerOnboardedOutbox は service 層と同じ shape の OutboxEvent を組み立てる。
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

// player-onboarded payload を Publisher 経由で送信し、subscriber まで bytes が
// そのまま届くことを固定する (outbox worker 送出経路の近似)。
func TestIntegration_PublishPlayerOnboarded(t *testing.T) {
	pub, topic := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, topic)

	ctx := context.Background()
	ev := buildPlayerOnboardedOutbox(t, "player-123", "Tenki")
	require.NoError(t, pub.Publish(ctx, ev.EventType, ev.Payload))

	msg, err := sub.WaitForMessage(ctx, 5*time.Second)
	require.NoError(t, err)

	var decoded apiscenario.PlayerOnboardedEvent
	require.NoError(t, json.Unmarshal(msg.Data, &decoded))

	assert.Equal(t, apiscenario.EventTypePlayerOnboarded, decoded.EventType)
	assert.Equal(t, ev.EventID.String(), decoded.EventID, "payload の event_id は outbox 行の PK と一致する")
	assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
	assert.Equal(t, "player-123", decoded.PlayerID)
	assert.Equal(t, "Tenki", decoded.InitialFactionID)
}

// 未登録 eventType への publish は Pub/Sub SDK 到達前に adapter 側で弾かれる
// 契約を固定する (outbox 行の eventType 設定ミスを worker の failure_count に
// 積ませるため)。
func TestIntegration_PublishUnknownEventType(t *testing.T) {
	pub, _ := setupPublisher(t)

	err := pub.Publish(context.Background(), "not-registered-event-type", []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown event type")
}

// publish が呼ばれなければ subscriber は timeout する (正例テストの偽陽性除け)。
func TestIntegration_NoPublish_SubscriberTimesOut(t *testing.T) {
	_, topic := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, topic)

	_, err := sub.WaitForMessage(context.Background(), 500*time.Millisecond)
	assert.ErrorIs(t, err, pubsubtest.ErrTimeout)
}
