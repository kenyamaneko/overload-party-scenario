package pubsub

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

var _ port.OutboxEventBuilder = (*EventBuilder)(nil)

// EventBuilder はドメインデータから outbox 行用の OutboxEvent を構築する。
// event struct のスキーマ (apiscenario.*) を知っているのは pubsub adapter のみに
// 閉じ込め、postgres adapter は payload を不透明な []byte として扱う。
//
// topic 名は Publisher と共通の設定値から渡す (enqueue 時と publish 時で不一致が
// 起きないようにするため)。
type EventBuilder struct {
	playerOnboardedTopic string
}

// NewEventBuilder は player-onboarded の送信先 topic 名を持つ EventBuilder を構築する。
func NewEventBuilder(playerOnboardedTopic string) (*EventBuilder, error) {
	if playerOnboardedTopic == "" {
		return nil, errors.New("pubsub: playerOnboardedTopic is required")
	}
	return &EventBuilder{
		playerOnboardedTopic: playerOnboardedTopic,
	}, nil
}

// BuildPlayerOnboarded は scenario のオンボーディング完了起因の player-onboarded
// イベントを構築する。subscriber (account) は display_name / onboarded フラグを
// 自スキーマに反映する。
func (b *EventBuilder) BuildPlayerOnboarded(playerID, displayName, initialFactionID string) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("pubsub: playerID is empty")
	}
	if displayName == "" {
		return port.OutboxEvent{}, errors.New("pubsub: displayName is empty")
	}
	if initialFactionID == "" {
		return port.OutboxEvent{}, errors.New("pubsub: initialFactionID is empty")
	}
	eventID := uuid.New()
	ev := apiscenario.PlayerOnboardedEvent{
		EventType:        apiscenario.EventTypePlayerOnboarded,
		EventID:          eventID.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		DisplayName:      displayName,
		InitialFactionID: initialFactionID,
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("marshal player-onboarded: %w", err)
	}
	return port.OutboxEvent{
		EventID: eventID,
		Topic:   b.playerOnboardedTopic,
		Payload: payload,
	}, nil
}
