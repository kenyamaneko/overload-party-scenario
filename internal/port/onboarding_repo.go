package port

import (
	"context"
	"errors"
	"time"
)

// ErrAlreadyOnboarded は同一 playerID が二度目のオンボーディング完了を試みたとき
// リポジトリ層が返す sentinel。scenario.player_onboarding の PRIMARY KEY
// 一意制約違反 (SQLSTATE 23505) を repo 層が classify してこのエラーに変換する。
// service 層はこれを受けて onboarding.ErrAlreadyOnboarded に翻訳し、
// handler は HTTP 409 Conflict にマップする。
var ErrAlreadyOnboarded = errors.New("onboarding: already completed")

// OnboardingStatus はプレイヤーのオンボーディング進捗を表す。
// Onboarded=false のとき CompletedAt は nil。
type OnboardingStatus struct {
	PlayerID    string
	Onboarded   bool
	CompletedAt *time.Time
}

// OnboardingRepo はオンボーディング完了状態の永続化を抽象化する。
// MarkComplete はビジネス行と outbox 行の両方を同一トランザクションで
// commit するため、dual-write 問題を発生させない契約になっている。
type OnboardingRepo interface {
	// GetStatus は指定プレイヤーのオンボーディング完了状態を返す。
	// 未完了なら Onboarded=false / CompletedAt=nil を返す (存在しないこと自体は
	// エラーではない)。DB 障害等の場合のみ error を返す。
	GetStatus(ctx context.Context, playerID string) (OnboardingStatus, error)

	// MarkComplete はビジネス行 (scenario.player_onboarding への INSERT) と
	// 任意個の outbox イベント行の INSERT を同一トランザクションで実行する。
	// 一意制約違反 (二度目以降の完了試行) の場合は ErrAlreadyOnboarded を返す。
	// events は可変長で受け、順に outbox に積む。
	MarkComplete(ctx context.Context, playerID string, events ...OutboxEvent) error
}
