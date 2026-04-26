// Package onboarding はオンボーディング (初回プロローグ) のユースケースを提供する。
// 各ステップの完了で対応する outbox event (onboarding-name-set / onboarding-faction-set /
// player-onboarded) を発行し、業務データの永続化は account 側 subscriber に委ねる。
// account への REST 呼び出しは表示名のバリデーションと完了 publish 用の
// faction 取得に限定し、書き込みは scenario 側で行わない。
package onboarding

import "errors"

var (
	// ErrAlreadyOnboarded はプレイヤーが既にオンボーディングを完了している場合に返される。
	// handler は HTTP 409 Conflict にマップする。
	ErrAlreadyOnboarded = errors.New("onboarding: already completed")

	// ErrScriptNotFound は要求言語のオンボーディングスクリプトがストアに存在しない場合に返される。
	// 代替言語へのフォールバックは行わない (CLAUDE.md の禁止事項)。
	// handler は HTTP 404 にマップする。
	ErrScriptNotFound = errors.New("onboarding: script not found")

	// ErrInvalidFaction は initial_faction_id が SelectableFactions に含まれない場合に返される。
	// handler は HTTP 400 にマップする。
	ErrInvalidFaction = errors.New("onboarding: invalid initial faction")

	// ErrInvalidName は name 入力ステップで account のバリデーションが失敗した場合に返される。
	// バリデーション SSoT は account に集約されており、scenario はそれを中継するだけ。
	// handler は HTTP 400 にマップする。
	ErrInvalidName = errors.New("onboarding: invalid name")

	// ErrPlayerNotFound は対象 playerID が account に存在しない場合に返される。
	// オンボード経路では Register 直後を前提とするため通常発生しないが、
	// 握りつぶさず構造的に表現する。handler は HTTP 404 にマップする。
	ErrPlayerNotFound = errors.New("onboarding: player not found")

	// ErrFactionNotSelected は完了 API が faction 選択ステップ未完了で叩かれた
	// フロー違反を表す。handler は HTTP 409 Conflict にマップする。
	ErrFactionNotSelected = errors.New("onboarding: initial faction not selected")
)
