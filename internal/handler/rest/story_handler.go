package rest

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/story"
)

// StoryHandler はシナリオ（ストーリー）の REST ハンドラを提供する。
type StoryHandler struct {
	svc *story.Service
}

// NewStoryHandler は StoryHandler を構築する。
func NewStoryHandler(svc *story.Service) *StoryHandler {
	return &StoryHandler{svc: svc}
}

// ListEpisodes はプレイヤー向けエピソード一覧をアンロック状態付きで返す。
func (h *StoryHandler) ListEpisodes(c *gin.Context) {
	playerID := c.Param("playerId")
	if playerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "playerId is required"})
		return
	}
	lang := c.DefaultQuery("lang", "ja")

	episodes, err := h.svc.ListEpisodes(c.Request.Context(), playerID, lang)
	if err != nil {
		slog.Error("list episodes failed", "error", err, "player_id", playerID, "lang", lang)
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"episodes": episodes})
}

// GetScript は指定エピソードのスクリプトを返す。
func (h *StoryHandler) GetScript(c *gin.Context) {
	playerID := c.Param("playerId")
	episodeID := c.Param("episodeId")
	lang := c.DefaultQuery("lang", "ja")

	script, err := h.svc.GetScript(c.Request.Context(), playerID, episodeID, lang)
	if err != nil {
		logGetScriptError(err, playerID, episodeID, lang)
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"episode_id": episodeID, "script": script})
}

// CompleteEpisode はエピソードの完了を記録する。
func (h *StoryHandler) CompleteEpisode(c *gin.Context) {
	playerID := c.Param("playerId")
	episodeID := c.Param("episodeId")

	if err := h.svc.CompleteEpisode(c.Request.Context(), playerID, episodeID); err != nil {
		if isNotFound(err) || isLocked(err) {
			slog.Info("complete episode rejected", "error", err, "player_id", playerID, "episode_id", episodeID)
		} else {
			slog.Error("complete episode failed", "error", err, "player_id", playerID, "episode_id", episodeID)
		}
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "episode completed", "episode_id": episodeID})
}

// logGetScriptError は GetScript のエラー種別ごとのログレベルを決定する。
func logGetScriptError(err error, playerID, episodeID, lang string) {
	attrs := []any{"error", err, "player_id", playerID, "episode_id", episodeID, "lang", lang}
	switch {
	case isNotFound(err), isLocked(err):
		slog.Info("get script rejected", attrs...)
	case isInfra(err):
		slog.Error("get script infra error", attrs...)
	default:
		slog.Error("get script failed", attrs...)
	}
}
