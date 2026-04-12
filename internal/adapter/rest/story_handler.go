package rest

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	"github.com/kenyamaneko/overload-party-scenario/internal/service"
)

// StoryHandler はシナリオ（ストーリー）の REST ハンドラを提供する。
type StoryHandler struct {
	storyService *service.StoryService
}

// NewStoryHandler は StoryService を受け取り StoryHandler を構築する。
func NewStoryHandler(storyService *service.StoryService) *StoryHandler {
	return &StoryHandler{storyService: storyService}
}

// ListEpisodes はプレイヤー向けエピソード一覧をロック状態付きで返す。
func (h *StoryHandler) ListEpisodes(c *gin.Context) {
	playerID := c.Param("playerId")
	if playerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "playerId is required"})
		return
	}
	lang := c.DefaultQuery("lang", "ja")

	episodes, err := h.storyService.ListEpisodes(c.Request.Context(), playerID, lang)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"episodes": episodes})
}

// GetScript は指定エピソードのスクリプトを返す。
func (h *StoryHandler) GetScript(c *gin.Context) {
	playerID := c.Param("playerId")
	episodeID := c.Param("episodeId")
	lang := c.DefaultQuery("lang", "ja")

	script, err := h.storyService.GetScript(c.Request.Context(), playerID, episodeID, lang)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEpisodeNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "episode not found"})
		case errors.Is(err, service.ErrEpisodeLocked):
			c.JSON(http.StatusForbidden, gin.H{"error": "episode is locked"})
		case errors.Is(err, port.ErrScriptNotFound):
			// 全対応ロケールでスクリプトファイルが存在しない。404 相当のため info レベル。
			log.Printf("scenario: script not found playerId=%s episodeId=%s lang=%s: %v",
				playerID, episodeID, lang, err)
			c.JSON(http.StatusNotFound, gin.H{"error": "script not found"})
		case errors.Is(err, port.ErrScriptInfra):
			// GCS / ファイルシステム障害 — 500 を返すが、wrap された原因はクライアントに漏らさない。
			log.Printf("scenario: script infra error playerId=%s episodeId=%s lang=%s: %v",
				playerID, episodeID, lang, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "script store unavailable"})
		default:
			log.Printf("scenario: unexpected GetScript error playerId=%s episodeId=%s: %v",
				playerID, episodeID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"episode_id": episodeID, "script": script})
}

// CompleteEpisode はエピソードの完了を記録する。
func (h *StoryHandler) CompleteEpisode(c *gin.Context) {
	playerID := c.Param("playerId")
	episodeID := c.Param("episodeId")

	err := h.storyService.CompleteEpisode(c.Request.Context(), playerID, episodeID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEpisodeNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrEpisodeLocked):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "episode completed", "episode_id": episodeID})
}
