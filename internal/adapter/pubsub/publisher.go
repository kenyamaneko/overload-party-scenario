// Package pubsub は scenario の Pub/Sub publisher を提供する。
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

// Publisher は eventType ごとに対応する Pub/Sub topic publisher を保持する。
type Publisher struct {
	client      *gpubsub.Client
	byEventType map[string]*gpubsub.Publisher
}

// New は eventType → topic mapping を構築する。
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

// Publish は eventType に対応する topic に payload を送出する (未登録 eventType はエラー)。
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
