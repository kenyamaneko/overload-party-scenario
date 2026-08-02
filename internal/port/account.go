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
	PlayerID       string
	InitialFaction *string
}

// PlayerProgress はエピソードのアンロック判定に使うプレイヤーの到達状況。
type PlayerProgress struct {
	Level         int64
	OwnedFactions []string
}

// OnboardingNameValidator はオンボーディング内 name 入力ステップの account 表示名バリデーションポート。
type OnboardingNameValidator interface {
	ValidateOnboardingName(ctx context.Context, name string) error
}

// OnboardingPlayerReader はオンボード Complete 時の初期 faction を account から取得するポート。
type OnboardingPlayerReader interface {
	GetOnboardingPlayer(ctx context.Context) (AccountPlayer, error)
}

// PlayerProgressReader はアンロック判定に必要な到達状況を account から取得するポート。
type PlayerProgressReader interface {
	GetPlayerProgress(ctx context.Context) (PlayerProgress, error)
}
