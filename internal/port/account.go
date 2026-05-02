package port

import (
	"context"
	"errors"
)

// ErrInvalidName は account の表示名バリデーション違反を表す sentinel。
var ErrInvalidName = errors.New("account: invalid name")

// ErrPlayerNotFound は対象 playerID が account に存在しないことを表す。
var ErrPlayerNotFound = errors.New("account: player not found")

// ErrFactionNotSelected はオンボード Complete 時点で account 側に初期 faction が未設定だった場合の sentinel。
var ErrFactionNotSelected = errors.New("account: initial faction not selected")

// AccountPlayer はオンボード Complete 時に scenario が account から取得する最小サブセット。
type AccountPlayer struct {
	PlayerID        string
	SelectedFaction *string
}

// OnboardingNameValidator はオンボーディング内 name 入力ステップの account 表示名バリデーションポート。
type OnboardingNameValidator interface {
	ValidateOnboardingName(ctx context.Context, playerID, name string) error
}

// OnboardingPlayerReader はオンボード Complete 時の選択 faction を account から取得するポート。
type OnboardingPlayerReader interface {
	GetOnboardingPlayer(ctx context.Context, playerID string) (AccountPlayer, error)
}
