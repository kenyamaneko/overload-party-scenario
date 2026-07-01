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
	"github.com/kenyamaneko/overload-party-scenario/internal/adapter/local"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/story"
)

const contractPlayerID = "11111111-1111-1111-1111-111111111111"

// newStoryEngine は GetScript の統合テスト用エンジンを組む。
func newStoryEngine(playerID, scriptRoot string) *gin.Engine {
	svc := story.New(postgres.NewStoryRepository(sharedPg.Pool), local.NewScriptStore(scriptRoot))
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

// TestStoryGetScriptContract は GetScript がエピソードの実状況に応じた応答契約を実 PostgreSQL で検証する。
func TestStoryGetScriptContract(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, scriptRoot string)
		episodeID  string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "未シードのエピソードを要求すると 404",
			setup:      func(t *testing.T, _ string) { seedPlayer(t, contractPlayerID, 10) },
			episodeID:  "missing-ep",
			wantStatus: http.StatusNotFound,
		},
		{
			name: "非アクティブなエピソードは 404",
			setup: func(t *testing.T, _ string) {
				seedPlayer(t, contractPlayerID, 10)
				seedEpisode(t, "ep-inactive", 1, "ep-inactive/{lang}.txt", false)
			},
			episodeID:  "ep-inactive",
			wantStatus: http.StatusNotFound,
		},
		{
			name: "required_level に満たないプレイヤーは 403",
			setup: func(t *testing.T, _ string) {
				seedPlayer(t, contractPlayerID, 4)
				seedEpisode(t, "ep-locked", 5, "ep-locked/{lang}.txt", true)
			},
			episodeID:  "ep-locked",
			wantStatus: http.StatusForbidden,
		},
		{
			name: "required_level == player level はアンロックされ 200 でスクリプトを返す",
			setup: func(t *testing.T, scriptRoot string) {
				seedPlayer(t, contractPlayerID, 5)
				seedEpisode(t, "ep-boundary", 5, "ep-boundary/{lang}.txt", true)
				writeScript(t, scriptRoot, "ep-boundary/ja.txt", "シナリオ本文")
			},
			episodeID:  "ep-boundary",
			wantStatus: http.StatusOK,
			wantBody:   "シナリオ本文",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sharedPg.Truncate(t)
			scriptRoot := t.TempDir()
			tt.setup(t, scriptRoot)

			req := httptest.NewRequest(http.MethodGet, "/episodes/"+tt.episodeID+"/script?lang=ja", nil)
			w := httptest.NewRecorder()
			newStoryEngine(contractPlayerID, scriptRoot).ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.wantBody)
		})
	}
}
