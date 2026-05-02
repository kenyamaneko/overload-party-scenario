package rest

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/story"
)

// errorStatus はドメインエラーを HTTP ステータスに分類する。
// usecase 層の sentinel を「not found / locked / infra」のいずれに翻訳するかは
// transport (handler) に閉じ、usecase からは HTTP を隠蔽する。
//
// default は 500 — DB 一時障害や未分類のエラーはクライアント側リトライに委ねる。
func errorStatus(err error) int {
	switch {
	case isNotFound(err):
		return http.StatusNotFound
	case isLocked(err):
		return http.StatusForbidden
	case isInfra(err):
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

func respondError(c *gin.Context, err error) {
	c.JSON(errorStatus(err), gin.H{"error": err.Error()})
}

// isNotFound はエピソードまたはスクリプトファイルが見つからないエラーか判定する。
// ErrScriptNotFound は「要求言語にスクリプトが存在しない」を指す（代替言語フォールバックはしない）。
func isNotFound(err error) bool {
	return errors.Is(err, story.ErrEpisodeNotFound) ||
		errors.Is(err, port.ErrScriptNotFound)
}

// isLocked はアンロック条件未達によりエピソードへアクセスできないエラーか判定する。
func isLocked(err error) bool {
	return errors.Is(err, story.ErrEpisodeLocked)
}

// isInfra はストレージ層の非 not-found 障害か判定する（GCS / ファイルシステムのネットワーク・権限エラー等）。
// 原因詳細はクライアントに漏らさず、ログのみに残す。
func isInfra(err error) bool {
	return errors.Is(err, port.ErrScriptInfra)
}
