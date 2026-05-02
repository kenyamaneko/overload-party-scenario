package port

import "context"

// RawEventPublisher は outbox worker が呼ぶ Pub/Sub 送出の低レベル interface。
type RawEventPublisher interface {
	Publish(ctx context.Context, eventType string, payload []byte) error
}
