package port

import (
	"context"
	"errors"
)

// ErrAlreadyOnboarded は同一 playerID が二度目のオンボーディング完了を試みたとき
// リポジトリ層が返す sentinel。scenario.player_onboarding の PRIMARY KEY
// 一意制約違反 (SQLSTATE 23505) を repo 層が classify してこのエラーに変換する。
// service 層はこれを受けて onboarding.ErrAlreadyOnboarded に翻訳し、
// handler は HTTP 409 Conflict にマップする。
var ErrAlreadyOnboarded = errors.New("onboarding: already completed")

// OnboardingRepo はオンボーディング進行イベントの永続化を抽象化する。
// MarkComplete は完了行と outbox 行を同一 tx で commit する。
// PublishEvents は scenario 側にビジネス書き込みのないステップ (name-set /
// faction-set) で outbox 行のみを単一 tx で commit する。
// どちらも dual-write 問題を発生させない契約。
type OnboardingRepo interface {
	// MarkComplete はビジネス行 (scenario.player_onboarding への INSERT) と
	// 任意個の outbox イベント行の INSERT を同一トランザクションで実行する。
	// 一意制約違反 (二度目以降の完了試行) の場合は ErrAlreadyOnboarded を返す。
	MarkComplete(ctx context.Context, playerID string, events ...OutboxEvent) error

	// PublishEvents は scenario 側にビジネス書き込みのないステップで、outbox 行を
	// 単一トランザクションで INSERT する。events が空なら no-op。
	PublishEvents(ctx context.Context, events ...OutboxEvent) error
}
