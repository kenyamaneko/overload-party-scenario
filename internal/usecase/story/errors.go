// Package story はストーリーエピソードのユースケースを提供する。
package story

import "errors"

var (
	// ErrEpisodeNotFound はエピソードが存在しないか非アクティブの場合に返される。
	ErrEpisodeNotFound = errors.New("episode not found")
	// ErrEpisodeLocked はアンロック条件を満たさないエピソードへのアクセスで返される。
	ErrEpisodeLocked = errors.New("episode is locked")
)
