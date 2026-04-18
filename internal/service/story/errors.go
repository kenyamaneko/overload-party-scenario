// Package story はストーリーエピソードのユースケース
// (エピソード一覧取得・スクリプト取得・完了記録・faction 通知) を提供する。
package story

import "errors"

var (
	// ErrEpisodeNotFound はエピソードが存在しないか非アクティブの場合に返される。
	ErrEpisodeNotFound = errors.New("episode not found")
	// ErrEpisodeLocked はアンロック条件を満たさないエピソードへのアクセスで返される。
	ErrEpisodeLocked = errors.New("episode is locked")
)
