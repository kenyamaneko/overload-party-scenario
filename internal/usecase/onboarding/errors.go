// Package onboarding はオンボーディング (初回プロローグ) のユースケースを提供する。
package onboarding

import "errors"

var (
	// ErrAlreadyOnboarded はプレイヤーが既にオンボーディングを完了している場合に返される。
	ErrAlreadyOnboarded = errors.New("onboarding: already completed")

	// ErrScriptNotFound は要求言語のオンボーディングスクリプトがストアに存在しない場合に返される。
	ErrScriptNotFound = errors.New("onboarding: script not found")

	// ErrInvalidFaction は initial_faction_id が SelectableFactions に含まれない場合に返される。
	ErrInvalidFaction = errors.New("onboarding: invalid initial faction")

	// ErrInvalidName は name 入力ステップで account のバリデーションが失敗した場合に返される。
	ErrInvalidName = errors.New("onboarding: invalid name")

	// ErrPlayerNotFound は対象 playerID が account に存在しない場合に返される。
	ErrPlayerNotFound = errors.New("onboarding: player not found")

	// ErrFactionNotSelected は完了 API が faction 選択ステップ未完了で叩かれたフロー違反を表す。
	ErrFactionNotSelected = errors.New("onboarding: initial faction not selected")
)
