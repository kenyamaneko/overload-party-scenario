package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-scenario/internal/adapter/rest"
)

// New は scenario の HTTP ルーターを構築する。
func New(storyH *rest.StoryHandler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	players := r.Group("/internal/v1/players/:playerId")
	{
		players.GET("/scenarios", storyH.ListEpisodes)
		players.GET("/scenarios/:episodeId/script", storyH.GetScript)
		players.POST("/scenarios/:episodeId/complete", storyH.CompleteEpisode)
	}
	return r
}
