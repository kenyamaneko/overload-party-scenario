package port

import (
	"context"
	"errors"
)

// ErrAlreadyOnboarded は同一 playerID が二度目のオンボーディング完了を試みたときにリポジトリが返す sentinel。
var ErrAlreadyOnboarded = errors.New("onboarding: already completed")

// OnboardingRepo はオンボーディング進行イベントの永続化を抽象化する。
type OnboardingRepo interface {
	// MarkComplete は player_onboarding 行と outbox イベント行を同一トランザクションで INSERT する。
	MarkComplete(ctx context.Context, playerID string, events ...OutboxEvent) error

	// PublishEvents は outbox 行のみを単一トランザクションで INSERT する (events が空なら no-op)。
	PublishEvents(ctx context.Context, events ...OutboxEvent) error
}
