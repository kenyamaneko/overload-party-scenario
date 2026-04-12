// Package pubsub は scenario サービスの Pub/Sub publisher を提供する。
//
// scenario はチュートリアルでプレイヤーの初期 faction を確定したとき
// `faction-selected` イベントを発行する。subscriber（account / card / gateway）が
// 副作用を処理する。
//
// scenario は subscriber を待たない。クライアント側 UX は account + card の commit 後に
// gateway WS push で駆動される。
package pubsub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	gpubsub "cloud.google.com/go/pubsub"
	"github.com/google/uuid"

	pubsubevents "github.com/kenyamaneko/overload-party-common/packages/pubsub-events"
)

// FactionPublisher は `faction-selected` topic 用の pubsub.Client + topic ハンドルを
// ラップする。port.FactionPublisher を実装する。
type FactionPublisher struct {
	client *gpubsub.Client
	topic  *gpubsub.Topic
}

// NewFactionPublisher は指定 Pub/Sub project + topic 名に紐づく publisher を構築する。
// topic は環境ごとに Terraform（modules/pubsub）で事前作成されている前提。
// topic ハンドルの検証に失敗した場合は fail-fast する。
func NewFactionPublisher(ctx context.Context, projectID, topicName string) (*FactionPublisher, error) {
	if projectID == "" {
		return nil, errors.New("pubsub: projectID is empty")
	}
	if topicName == "" {
		return nil, errors.New("pubsub: topicName is empty")
	}
	client, err := gpubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub: new pubsub client: %w", err)
	}
	topic := client.Topic(topicName)
	ok, err := topic.Exists(ctx)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("pubsub: check topic %q: %w", topicName, err)
	}
	if !ok {
		_ = client.Close()
		return nil, fmt.Errorf("pubsub: topic %q does not exist in project %q", topicName, projectID)
	}
	return &FactionPublisher{client: client, topic: topic}, nil
}

// Close は topic を停止（in-flight メッセージを flush）し Pub/Sub client を閉じる。
// 複数回呼び出しても安全。
func (p *FactionPublisher) Close() error {
	p.topic.Stop()
	return p.client.Close()
}

// PublishFactionSelected は scenario の初期 faction 選択フロー用に faction-selected
// イベントを発行する。Source は FactionSourceScenarioInitial に固定（shop は独自の
// publisher を使用する）。
// Pub/Sub ack を待ってブロックする。失敗時は REST handler にエラーを返し、
// クライアントが通常フローでリトライする。
func (p *FactionPublisher) PublishFactionSelected(ctx context.Context, playerID, faction string) error {
	if playerID == "" {
		return errors.New("pubsub: playerID is empty")
	}
	if faction == "" {
		return errors.New("pubsub: faction is empty")
	}
	ev := pubsubevents.FactionSelectedEvent{
		EventType: pubsubevents.EventTypeFactionSelected,
		EventID:   uuid.NewString(),
		Timestamp: time.Now().UTC(),
		PlayerID:  playerID,
		Faction:   faction,
		Source:    pubsubevents.FactionSourceScenarioInitial,
	}
	return publishJSON(ctx, p.topic, ev)
}

func publishJSON(ctx context.Context, topic *gpubsub.Topic, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("pubsub: marshal %T: %w", event, err)
	}
	result := topic.Publish(ctx, &gpubsub.Message{Data: data})
	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("pubsub: publish to %s: %w", topic.ID(), err)
	}
	return nil
}
