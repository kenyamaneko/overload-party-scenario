//go:build integration

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	adapterhttp "github.com/kenyamaneko/overload-party-scenario/internal/adapter/http"
	"github.com/kenyamaneko/overload-party-scenario/internal/adapter/local"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/story"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// newStoryEpisodesEngine はエピソード一覧・完了の統合テスト用エンジンを組む。
func newStoryEpisodesEngine(playerID, scriptRoot, accountBaseURL string) *gin.Engine {
	svc := story.New(
		postgres.NewStoryRepository(sharedPg.Pool),
		local.NewScriptStore(scriptRoot),
		adapterhttp.NewAccountClient(accountBaseURL),
	)
	h := NewStoryHandler(svc)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(internalauth.PlayerIDContextKey, playerID) })
	r.GET("/episodes", h.ListEpisodes)
	r.POST("/episodes/:episodeId/complete", h.CompleteEpisode)
	return r
}

// countCompletedEpisodes は player_story_progress の完了記録行数を返す。
func countCompletedEpisodes(t *testing.T, playerID string) int {
	t.Helper()
	var n int
	require.NoError(t, sharedPg.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM scenario.player_story_progress WHERE player_id = $1`, playerID).Scan(&n))
	return n
}

func TestStoryListEpisodesContract(t *testing.T) {
	t.Run("エピソード一覧取得の応答契約", func(t *testing.T) {
		tests := []struct {
			name          string
			level         int64
			ownedFactions []string
			setup         func(t *testing.T, engine *gin.Engine)
			wantStatus    int
			verify        func(t *testing.T, body apiscenario.EpisodesListResponse)
		}{
			{
				name:       "エピソードが1件も無いとき、200で一覧は空になる",
				level:      10,
				setup:      func(t *testing.T, _ *gin.Engine) {},
				wantStatus: http.StatusOK,
				verify: func(t *testing.T, body apiscenario.EpisodesListResponse) {
					assert.Empty(t, body.Episodes)
				},
			},
			{
				name:  "プレイヤーのレベルが必要レベルにちょうど達しているとき、そのエピソードはアンロック済みで返る",
				level: 5,
				setup: func(t *testing.T, _ *gin.Engine) {
					seedEpisode(t, "ep-boundary", 5, "ep-boundary/{lang}.txt", true)
				},
				wantStatus: http.StatusOK,
				verify: func(t *testing.T, body apiscenario.EpisodesListResponse) {
					require.Len(t, body.Episodes, 1)
					assert.True(t, body.Episodes[0].IsUnlocked)
					assert.Empty(t, body.Episodes[0].LockReasons)
				},
			},
			{
				name:  "プレイヤーのレベルが必要レベルに1足りないとき、レベル不足のロック理由付きで返る",
				level: 4,
				setup: func(t *testing.T, _ *gin.Engine) {
					seedEpisode(t, "ep-locked", 5, "ep-locked/{lang}.txt", true)
				},
				wantStatus: http.StatusOK,
				verify: func(t *testing.T, body apiscenario.EpisodesListResponse) {
					require.Len(t, body.Episodes, 1)
					require.Len(t, body.Episodes[0].LockReasons, 1)
					reason := body.Episodes[0].LockReasons[0]
					assert.Equal(t, apiscenario.LockReasonType("level"), reason.Type)
					require.NotNil(t, reason.RequiredLevel)
					assert.Equal(t, int64(5), *reason.RequiredLevel)
					require.NotNil(t, reason.CurrentLevel)
					assert.Equal(t, int64(4), *reason.CurrentLevel)
				},
			},
			{
				name:          "必要ファクションを保有しているとき、そのエピソードはアンロック済みで返る",
				level:         10,
				ownedFactions: []string{"SHE"},
				setup: func(t *testing.T, _ *gin.Engine) {
					seedEpisode(t, "ep-faction", 1, "ep-faction/{lang}.txt", true)
					seedRequiredFaction(t, "ep-faction", "SHE")
				},
				wantStatus: http.StatusOK,
				verify: func(t *testing.T, body apiscenario.EpisodesListResponse) {
					require.Len(t, body.Episodes, 1)
					assert.True(t, body.Episodes[0].IsUnlocked)
					assert.Empty(t, body.Episodes[0].LockReasons)
				},
			},
			{
				name:  "必要ファクションを保有していないとき、ファクション不足のロック理由付きで返る",
				level: 10,
				setup: func(t *testing.T, _ *gin.Engine) {
					seedEpisode(t, "ep-faction-missing", 1, "ep-faction-missing/{lang}.txt", true)
					seedRequiredFaction(t, "ep-faction-missing", "SHE")
				},
				wantStatus: http.StatusOK,
				verify: func(t *testing.T, body apiscenario.EpisodesListResponse) {
					require.Len(t, body.Episodes, 1)
					assert.False(t, body.Episodes[0].IsUnlocked)
					require.Len(t, body.Episodes[0].LockReasons, 1)
					reason := body.Episodes[0].LockReasons[0]
					assert.Equal(t, apiscenario.LockReasonType("faction"), reason.Type)
					require.NotNil(t, reason.RequiredFaction)
					assert.Equal(t, "SHE", *reason.RequiredFaction)
				},
			},
			{
				name:  "非アクティブのエピソードは一覧に含まれない",
				level: 10,
				setup: func(t *testing.T, _ *gin.Engine) {
					seedEpisode(t, "ep-inactive", 1, "ep-inactive/{lang}.txt", false)
				},
				wantStatus: http.StatusOK,
				verify: func(t *testing.T, body apiscenario.EpisodesListResponse) {
					assert.Empty(t, body.Episodes)
				},
			},
			{
				name:  "完了させたエピソードは、その後の一覧で完了済み印が付く",
				level: 10,
				setup: func(t *testing.T, engine *gin.Engine) {
					seedEpisode(t, "ep-complete", 1, "ep-complete/{lang}.txt", true)
					w := httptest.NewRecorder()
					engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/episodes/ep-complete/complete", nil))
					require.Equal(t, http.StatusOK, w.Code)
				},
				wantStatus: http.StatusOK,
				verify: func(t *testing.T, body apiscenario.EpisodesListResponse) {
					require.Len(t, body.Episodes, 1)
					assert.True(t, body.Episodes[0].IsCompleted)
				},
			},
			{
				name:  "複数エピソードは表示順の設定どおりに並んで返る",
				level: 10,
				setup: func(t *testing.T, _ *gin.Engine) {
					seedEpisodeWithOrder(t, "ep-zulu", 2, 1, "ep-zulu/{lang}.txt", 1, true)
					seedEpisodeWithOrder(t, "ep-alpha", 1, 1, "ep-alpha/{lang}.txt", 2, true)
				},
				wantStatus: http.StatusOK,
				verify: func(t *testing.T, body apiscenario.EpisodesListResponse) {
					require.Len(t, body.Episodes, 2)
					assert.Equal(t, "ep-zulu", body.Episodes[0].EpisodeID)
					assert.Equal(t, "ep-alpha", body.Episodes[1].EpisodeID)
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				account := newAccountFake(t, tt.level, tt.ownedFactions...)
				engine := newStoryEpisodesEngine(contractPlayerID, t.TempDir(), account.URL())
				tt.setup(t, engine)

				w := httptest.NewRecorder()
				engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/episodes", nil))

				assert.Equal(t, tt.wantStatus, w.Code)
				var body apiscenario.EpisodesListResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				tt.verify(t, body)
			})
		}

		t.Run("accountが応答しないとき、500とその理由を返し、accountが復旧すると一覧を返す", func(t *testing.T) {
			sharedPg.Truncate(t)
			account := newAccountFake(t, 10)
			engine := newStoryEpisodesEngine(contractPlayerID, t.TempDir(), account.URL())
			seedEpisode(t, "ep-account-down", 1, "ep-account-down/{lang}.txt", true)

			respondPlayer := account.GetPlayerFn
			account.GetPlayerFn = func() (int, any) { return http.StatusInternalServerError, nil }

			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/episodes", nil))
			require.Equal(t, http.StatusInternalServerError, w.Code)
			assert.Contains(t, w.Body.String(), "get player progress")

			account.GetPlayerFn = respondPlayer

			w = httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/episodes", nil))
			require.Equal(t, http.StatusOK, w.Code)
			var body apiscenario.EpisodesListResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Len(t, body.Episodes, 1)
			assert.True(t, body.Episodes[0].IsUnlocked)
		})
	})
}

func TestStoryCompleteEpisodeContract(t *testing.T) {
	t.Run("エピソード完了の応答契約", func(t *testing.T) {
		tests := []struct {
			name              string
			level             int64
			setup             func(t *testing.T, engine *gin.Engine)
			episodeID         string
			wantStatus        int
			wantCompletion    int
			wantBodyEpisodeID string
			wantBodyContains  string
		}{
			{
				name:  "アンロック済みのエピソードを完了すると、200になり完了記録が1件残る",
				level: 10,
				setup: func(t *testing.T, _ *gin.Engine) {
					seedEpisode(t, "ep-unlocked", 1, "ep-unlocked/{lang}.txt", true)
				},
				episodeID:         "ep-unlocked",
				wantStatus:        http.StatusOK,
				wantCompletion:    1,
				wantBodyEpisodeID: "ep-unlocked",
				wantBodyContains:  "episode completed",
			},
			{
				name:  "同じエピソードを二度完了しても、200のまま完了記録は1件に保たれる",
				level: 10,
				setup: func(t *testing.T, engine *gin.Engine) {
					seedEpisode(t, "ep-repeat", 1, "ep-repeat/{lang}.txt", true)
					w := httptest.NewRecorder()
					engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/episodes/ep-repeat/complete", nil))
					require.Equal(t, http.StatusOK, w.Code)
				},
				episodeID:         "ep-repeat",
				wantStatus:        http.StatusOK,
				wantCompletion:    1,
				wantBodyEpisodeID: "ep-repeat",
				wantBodyContains:  "episode completed",
			},
			{
				name:              "存在しないエピソードを完了しようとすると、404になり記録は残らない",
				level:             10,
				setup:             func(t *testing.T, _ *gin.Engine) {},
				episodeID:         "missing-ep",
				wantStatus:        http.StatusNotFound,
				wantCompletion:    0,
				wantBodyEpisodeID: "",
				wantBodyContains:  "episode not found",
			},
			{
				name:  "非アクティブのエピソードを完了しようとすると、404になり記録は残らない",
				level: 10,
				setup: func(t *testing.T, _ *gin.Engine) {
					seedEpisode(t, "ep-inactive-complete", 1, "ep-inactive-complete/{lang}.txt", false)
				},
				episodeID:         "ep-inactive-complete",
				wantStatus:        http.StatusNotFound,
				wantCompletion:    0,
				wantBodyEpisodeID: "",
				wantBodyContains:  "episode not found",
			},
			{
				name:  "レベル不足でロック中のエピソードを完了しようとすると、403になり記録は残らない",
				level: 1,
				setup: func(t *testing.T, _ *gin.Engine) {
					seedEpisode(t, "ep-too-high", 5, "ep-too-high/{lang}.txt", true)
				},
				episodeID:         "ep-too-high",
				wantStatus:        http.StatusForbidden,
				wantCompletion:    0,
				wantBodyEpisodeID: "",
				wantBodyContains:  "episode is locked",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				account := newAccountFake(t, tt.level)
				engine := newStoryEpisodesEngine(contractPlayerID, t.TempDir(), account.URL())
				tt.setup(t, engine)

				w := httptest.NewRecorder()
				engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/episodes/"+tt.episodeID+"/complete", nil))

				assert.Equal(t, tt.wantStatus, w.Code)
				assert.Equal(t, tt.wantCompletion, countCompletedEpisodes(t, contractPlayerID))
				assert.Contains(t, w.Body.String(), tt.wantBodyContains)
				var body apiscenario.ScenarioCompleteResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				assert.Equal(t, tt.wantBodyEpisodeID, body.EpisodeID)
			})
		}
	})
}
