//go:build integration

package rest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	adapterhttp "github.com/kenyamaneko/overload-party-scenario/internal/adapter/http"
	"github.com/kenyamaneko/overload-party-scenario/internal/adapter/local"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/story"
)

const contractPlayerID = "11111111-1111-1111-1111-111111111111"

// newStoryEngine は GetScript の統合テスト用エンジンを組む。
func newStoryEngine(playerID, scriptRoot, accountBaseURL string) *gin.Engine {
	svc := story.New(
		postgres.NewStoryRepository(sharedPg.Pool),
		local.NewScriptStore(scriptRoot),
		adapterhttp.NewAccountClient(accountBaseURL),
	)
	h := NewStoryHandler(svc)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(internalauth.PlayerIDContextKey, playerID) })
	r.GET("/episodes/:episodeId/script", h.GetScript)
	return r
}

// writeScript は scriptRoot 配下に 1 スクリプトファイルを書き出す。
func writeScript(t *testing.T, scriptRoot, key, body string) {
	t.Helper()
	full := filepath.Join(scriptRoot, key)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
}

func TestStoryGetScriptContract(t *testing.T) {
	t.Run("エピソードスクリプト取得の応答契約", func(t *testing.T) {
		tests := []struct {
			name          string
			level         int64
			ownedFactions []string
			setup         func(t *testing.T, scriptRoot string)
			episodeID     string
			wantStatus    int
			wantBody      string
		}{
			{
				name:       "未シードのエピソードを要求するとき、404になる",
				level:      10,
				setup:      func(t *testing.T, _ string) {},
				episodeID:  "missing-ep",
				wantStatus: http.StatusNotFound,
			},
			{
				name:  "非アクティブなエピソードのとき、404になる",
				level: 10,
				setup: func(t *testing.T, _ string) {
					seedEpisode(t, "ep-inactive", 1, "ep-inactive/{lang}.txt", false)
				},
				episodeID:  "ep-inactive",
				wantStatus: http.StatusNotFound,
			},
			{
				name:  "required_levelに満たないプレイヤーのとき、403になる",
				level: 4,
				setup: func(t *testing.T, _ string) {
					seedEpisode(t, "ep-locked", 5, "ep-locked/{lang}.txt", true)
				},
				episodeID:  "ep-locked",
				wantStatus: http.StatusForbidden,
			},
			{
				name:  "required_level == player levelのとき、アンロックされ200でスクリプトを返す",
				level: 5,
				setup: func(t *testing.T, scriptRoot string) {
					seedEpisode(t, "ep-boundary", 5, "ep-boundary/{lang}.txt", true)
					writeScript(t, scriptRoot, "ep-boundary/ja.txt", "シナリオ本文")
				},
				episodeID:  "ep-boundary",
				wantStatus: http.StatusOK,
				wantBody:   "シナリオ本文",
			},
			{
				name:  "アンロック済みのエピソードのスクリプトファイルが無いとき、404になる",
				level: 5,
				setup: func(t *testing.T, _ string) {
					seedEpisode(t, "ep-no-script", 5, "ep-no-script/{lang}.txt", true)
				},
				episodeID:  "ep-no-script",
				wantStatus: http.StatusNotFound,
				wantBody:   "story script not found",
			},
			{
				name:  "スクリプトの保存先が読み取れない状態のとき、500になる",
				level: 5,
				setup: func(t *testing.T, scriptRoot string) {
					seedEpisode(t, "ep-infra", 5, "ep-infra/{lang}.txt", true)
					require.NoError(t, os.MkdirAll(filepath.Join(scriptRoot, "ep-infra/ja.txt"), 0o755))
				},
				episodeID:  "ep-infra",
				wantStatus: http.StatusInternalServerError,
				wantBody:   "story script infrastructure error",
			},
			{
				name:  "必須陣営を所有していないとき、スクリプト取得は403になる",
				level: 5,
				setup: func(t *testing.T, _ string) {
					seedEpisode(t, "ep-faction-locked", 1, "ep-faction-locked/{lang}.txt", true)
					seedRequiredFaction(t, "ep-faction-locked", "SHE")
				},
				episodeID:  "ep-faction-locked",
				wantStatus: http.StatusForbidden,
				wantBody:   "episode is locked",
			},
			{
				name:          "必須陣営を所有しているとき、200でスクリプトを返す",
				level:         5,
				ownedFactions: []string{"SHE"},
				setup: func(t *testing.T, scriptRoot string) {
					seedEpisode(t, "ep-faction-unlocked", 1, "ep-faction-unlocked/{lang}.txt", true)
					seedRequiredFaction(t, "ep-faction-unlocked", "SHE")
					writeScript(t, scriptRoot, "ep-faction-unlocked/ja.txt", "陣営本文")
				},
				episodeID:  "ep-faction-unlocked",
				wantStatus: http.StatusOK,
				wantBody:   "陣営本文",
			},
			{
				name:  "前提エピソードが未完了のとき、スクリプト取得は403になる",
				level: 5,
				setup: func(t *testing.T, _ string) {
					seedEpisodeWithRequiredEpisodes(t, "ep-prereq-locked", 1, "ep-prereq-locked/{lang}.txt", true, []string{"ep-prereq"})
				},
				episodeID:  "ep-prereq-locked",
				wantStatus: http.StatusForbidden,
				wantBody:   "episode is locked",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				scriptRoot := t.TempDir()
				account := newAccountFake(t, tt.level, tt.ownedFactions...)
				tt.setup(t, scriptRoot)

				req := httptest.NewRequest(http.MethodGet, "/episodes/"+tt.episodeID+"/script?lang=ja", nil)
				w := httptest.NewRecorder()
				newStoryEngine(contractPlayerID, scriptRoot, account.URL()).ServeHTTP(w, req)

				assert.Equal(t, tt.wantStatus, w.Code)
				assert.Contains(t, w.Body.String(), tt.wantBody)
			})
		}

		t.Run("langを指定しないとき、日本語のスクリプトが返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			scriptRoot := t.TempDir()
			account := newAccountFake(t, 5)
			seedEpisode(t, "ep-default-lang", 5, "ep-default-lang/{lang}.txt", true)
			writeScript(t, scriptRoot, "ep-default-lang/ja.txt", "日本語本文")
			writeScript(t, scriptRoot, "ep-default-lang/en.txt", "english body")

			req := httptest.NewRequest(http.MethodGet, "/episodes/ep-default-lang/script", nil)
			w := httptest.NewRecorder()
			newStoryEngine(contractPlayerID, scriptRoot, account.URL()).ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), "日本語本文")
		})
	})
}
