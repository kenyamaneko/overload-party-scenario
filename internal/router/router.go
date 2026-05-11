package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-scenario/internal/handler/rest"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

// New は scenario の HTTP ルーターを構築する。
// gateway 経由の player-scoped API は X-Internal-Auth (HS256 JWT) で認証し、
// sub クレームを context に注入することで handler が path/body の player_id を扱わずに済む。
func New(
	storyH *rest.StoryHandler,
	onboardingH *rest.OnboardingHandler,
	authVerifier port.InternalAuthVerifier,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1/scenarios")
	api.Use(rest.VerifyInternalAuth(authVerifier))
	{
		api.GET("/episodes", storyH.ListEpisodes)
		api.GET("/episodes/:episodeId/script", storyH.GetScript)
		api.POST("/episodes/:episodeId/complete", storyH.CompleteEpisode)

		api.GET("/onboarding/script", onboardingH.GetScript)
		api.PUT("/onboarding/name", onboardingH.UpdateName)
		api.POST("/onboarding/faction", onboardingH.SelectFaction)
		api.POST("/onboarding/complete", onboardingH.Complete)
	}
	return r
}
