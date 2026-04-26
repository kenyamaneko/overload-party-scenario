// Package onboarding はオンボーディング (初回プロローグ) のユースケースを提供する。
// status 取得・script 取得・完了記録 + outbox enqueue を束ね、initial_faction_id は
// account / card / gateway に Transactional Outbox 経由で伝播させる。
// 表示名はオンボード内 name 入力ステップで account に対し REST 同期書込で確定するため、
// 本パッケージでは扱わない。
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
)
