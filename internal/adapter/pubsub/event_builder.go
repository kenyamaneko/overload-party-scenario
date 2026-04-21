package pubsub

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	pubsubevents "github.com/kenyamaneko/overload-party-common/packages/pubsub-events"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

var _ port.OutboxEventBuilder = (*EventBuilder)(nil)

// EventBuilder はドメインデータから outbox 行用の OutboxEvent を構築する。
// event struct のスキーマ (pubsubevents.* / apiscenario.*) を知っているのは
// pubsub adapter のみに閉じ込め、postgres adapter は payload を不透明な []byte として扱う。
//
// topic 名は Publisher と共通の設定値から渡す (enqueue 時と publish 時で不一致が
// 起きないようにするため)。
type EventBuilder struct {
	factionSelectedTopic string
	playerOnboardedTopic string
}

// NewEventBuilder は各イベントの送信先 topic 名を持つ EventBuilder を構築する。
func NewEventBuilder(factionSelectedTopic, playerOnboardedTopic string) (*EventBuilder, error) {
	if factionSelectedTopic == "" || playerOnboardedTopic == "" {
		return nil, errors.New("pubsub: both topic names are required")
	}
	return &EventBuilder{
		factionSelectedTopic: factionSelectedTopic,
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

// BuildFactionSelected は scenario のオンボーディング完了起因の faction-selected
// イベントを構築する。Source は常に FactionSourceScenarioInitial (shop 購入起因の
// イベントは別サービスが発行する)。
func (b *EventBuilder) BuildFactionSelected(playerID, faction string) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("pubsub: playerID is empty")
	}
	if faction == "" {
		return port.OutboxEvent{}, errors.New("pubsub: faction is empty")
	}
	eventID := uuid.New()
	ev := pubsubevents.FactionSelectedEvent{
		EventType: pubsubevents.EventTypeFactionSelected,
		EventID:   eventID.String(),
		Timestamp: time.Now().UTC(),
		PlayerID:  playerID,
		Faction:   faction,
		Source:    pubsubevents.FactionSourceScenarioInitial,
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("marshal faction-selected: %w", err)
	}
	return port.OutboxEvent{
		EventID: eventID,
		Topic:   b.factionSelectedTopic,
		Payload: payload,
	}, nil
}
