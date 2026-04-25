// Package pubsub は scenario の Pub/Sub publisher。worker (outbox) から呼ばれる
// 低レベル送信層で、論理 eventType を物理 topic に解決して送出する。
//
// scenario が発行するサービス横断イベント:
//
//   - apiscenario.EventTypePlayerOnboarded — オンボーディング完了時に発行。account が
//     subscribe して display_name の永続化とオンボード済みフラグ立てを行う。
//     その他の subscriber (card / gateway など) も本イベントから faction 状態を
//     同期する (ADR-022)。
package pubsub

import (
	"context"
	"errors"
	"fmt"

	gpubsub "cloud.google.com/go/pubsub/v2"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

var _ port.RawEventPublisher = (*Publisher)(nil)

// Publisher は port.RawEventPublisher を実装する。
type Publisher struct {
	client      *gpubsub.Client
	byEventType map[string]*gpubsub.Publisher
}

// New は物理 topic 名から eventType→topic mapping を構築する。topic 名は
// configmap / env で外から差し替えできるよう構築時に受け取る。topic は
// Terraform (modules/pubsub) で事前作成されている前提。
func New(ctx context.Context, projectID, playerOnboardedTopic string) (*Publisher, error) {
	if projectID == "" {
		return nil, errors.New("pubsub: projectID is empty")
	}
	if playerOnboardedTopic == "" {
		return nil, errors.New("pubsub: playerOnboardedTopic is required")
	}
	client, err := gpubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub: new pubsub client: %w", err)
	}
	return &Publisher{
		client: client,
		byEventType: map[string]*gpubsub.Publisher{
			apiscenario.EventTypePlayerOnboarded: client.Publisher(playerOnboardedTopic),
		},
	}, nil
}

// Close は in-flight メッセージを flush し Pub/Sub client を閉じる。
func (p *Publisher) Close() error {
	for _, pub := range p.byEventType {
		pub.Stop()
	}
	return p.client.Close()
}

// Publish は未登録 eventType をエラーで返し、outbox 行の設定ミスを alert
// 経路に載せる (Pub/Sub SDK に届く前に失敗させる)。
func (p *Publisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	pub, ok := p.byEventType[eventType]
	if !ok {
		return fmt.Errorf("pubsub: unknown event type %q", eventType)
	}
	result := pub.Publish(ctx, &gpubsub.Message{Data: payload})
	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("pubsub: publish event_type=%s: %w", eventType, err)
	}
	return nil
}
