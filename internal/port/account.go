package port

import (
	"context"
	"errors"
)

// ErrInvalidName は account の表示名バリデーション違反を表す sentinel。
// 表示名のバリデーション SSoT は account に集約されており、accountclient adapter が
// account の 400 レスポンスを本 sentinel に翻訳して service 層へ渡す。
var ErrInvalidName = errors.New("account: invalid name")

// ErrPlayerNotFound は対象 playerID が account に存在しないことを表す。
// オンボード再開判定では Register 直後を前提とするため通常発生しないが、
// adapter 層で 404 を握りつぶさず構造的に表現する。
var ErrPlayerNotFound = errors.New("account: player not found")

// AccountPlayer は再開判定で必要となる account.Player の最小サブセット。
// 表示名と選択中 faction の nullable 状態だけが checkpoint 導出に効く。
// 汎用 Player 全体を scenario に持ち込まないことを構造的に表す。
type AccountPlayer struct {
	PlayerID        string
	Name            *string
	SelectedFaction *string
}

// OnboardingNameUpdater はオンボーディング内 name 入力ステップ専用の
// account 表示名確定ポート。
// account の業務バリデーションを SSoT とし、違反は ErrInvalidName で返す。
type OnboardingNameUpdater interface {
	UpdateOnboardingName(ctx context.Context, playerID, name string) error
}

// OnboardingPlayerReader はオンボーディング再開判定専用の account 状態取得ポート。
// 返す情報は AccountPlayer に絞られ、汎用 Player 取得には使わない。
type OnboardingPlayerReader interface {
	GetOnboardingPlayer(ctx context.Context, playerID string) (AccountPlayer, error)
}
