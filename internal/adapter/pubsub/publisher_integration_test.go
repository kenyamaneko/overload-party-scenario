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

// TestMain は package 内の全 integration test で共有する emulator を起動する。
// container 起動コストは高いので per-test ではなく package scope で償却する。
// test 毎の分離は topic / subscription の UUID suffix で担保する。
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

// Publisher を emulator に向けて構築するヘルパ。topic を事前作成。
func setupPublisher(t *testing.T) (*Publisher, string) {
	t.Helper()
	topic := sharedEmulator.CreateTopic(t, apiscenario.TopicPlayerOnboarded)

	ctx := context.Background()
	pub, err := New(ctx, sharedEmulator.ProjectID(), topic)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pub.Close() })

	return pub, topic
}

// buildPlayerOnboardedOutbox は service 層が outbox 行を組み立てるのと同じ shape の
// OutboxEvent を作るテストヘルパ (shop の buildFactionPurchasedOutbox に対応)。
// EventType は apiscenario の定数に固定し、payload は本番と同じ JSON Marshal 結果。
func buildPlayerOnboardedOutbox(t *testing.T, playerID, displayName, initialFactionID string) port.OutboxEvent {
	t.Helper()
	id := uuid.New()
	payload, err := json.Marshal(apiscenario.PlayerOnboardedEvent{
		EventType:        apiscenario.EventTypePlayerOnboarded,
		EventID:          id.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		DisplayName:      displayName,
		InitialFactionID: initialFactionID,
	})
	require.NoError(t, err)
	return port.OutboxEvent{
		EventID:   id,
		EventType: apiscenario.EventTypePlayerOnboarded,
		Payload:   payload,
	}
}

// player-onboarded payload が Publisher で送信できる shape であることを固定する
// (outbox 行を worker が送出する経路の近似)。
func TestIntegration_PublishPlayerOnboarded(t *testing.T) {
	pub, topic := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, topic)

	ctx := context.Background()
	ev := buildPlayerOnboardedOutbox(t, "player-123", "Tenki Lover", "Tenki")
	require.NoError(t, pub.Publish(ctx, ev.EventType, ev.Payload))

	msg, err := sub.WaitForMessage(ctx, 5*time.Second)
	require.NoError(t, err)

	var decoded apiscenario.PlayerOnboardedEvent
	require.NoError(t, json.Unmarshal(msg.Data, &decoded))

	assert.Equal(t, apiscenario.EventTypePlayerOnboarded, decoded.EventType)
	assert.Equal(t, ev.EventID.String(), decoded.EventID, "payload の event_id は outbox 行の PK と一致する")
	assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
	assert.Equal(t, "player-123", decoded.PlayerID)
	assert.Equal(t, "Tenki Lover", decoded.DisplayName)
	assert.Equal(t, "Tenki", decoded.InitialFactionID)
}

// 未登録 eventType への publish は (emulator 越しでも) Pub/Sub SDK 到達前に
// adapter 側で弾かれることを固定する。outbox 行の eventType 設定ミスを worker の
// failure_count に積ませる経路を担保する。
func TestIntegration_PublishUnknownEventType(t *testing.T) {
	pub, _ := setupPublisher(t)

	err := pub.Publish(context.Background(), "not-registered-event-type", []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown event type")
}

// negative path: publish が呼ばれなければ subscriber は timeout する。
// 「何も起きないこと」を肯定的に検証することで、正例テストの偽陽性 (別経路で
// たまたまメッセージが届いている) を排除する。
func TestIntegration_NoPublish_SubscriberTimesOut(t *testing.T) {
	_, topic := setupPublisher(t)
	sub := sharedEmulator.Subscribe(t, topic)

	_, err := sub.WaitForMessage(context.Background(), 500*time.Millisecond)
	assert.ErrorIs(t, err, pubsubtest.ErrTimeout)
}
