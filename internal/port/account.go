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
// オンボード経路では Register 直後を前提とするため通常発生しないが、
// adapter 層で 404 を握りつぶさず構造的に表現する。
var ErrPlayerNotFound = errors.New("account: player not found")

// ErrFactionNotSelected はオンボード Complete 時点で account 側に初期 faction が
// 未設定だった場合に accountclient adapter が返す sentinel。faction 選択ステップを
// 経ずに完了 API が叩かれたフロー違反を構造的に表現する。
var ErrFactionNotSelected = errors.New("account: initial faction not selected")

// AccountPlayer はオンボード Complete 時に scenario が account から取得する最小サブセット。
// 完了 publish ペイロード (PlayerOnboardedEvent.InitialFactionID) を組み立てるために
// 選択 faction が必要となる。
type AccountPlayer struct {
	PlayerID        string
	SelectedFaction *string
}

// OnboardingNameValidator はオンボーディング内 name 入力ステップ専用の
// account 表示名バリデーションポート。書き込みは行わず、4xx を即時にユーザーへ返すための
// 同期確認のみを担う。書き込みは onboarding-name-set event subscriber が後段で実行する。
type OnboardingNameValidator interface {
	ValidateOnboardingName(ctx context.Context, playerID, name string) error
}

// OnboardingPlayerReader はオンボード Complete 時に PlayerOnboardedEvent payload に詰める
// 選択 faction を account から取得するためのポート。
type OnboardingPlayerReader interface {
	GetOnboardingPlayer(ctx context.Context, playerID string) (AccountPlayer, error)
}
