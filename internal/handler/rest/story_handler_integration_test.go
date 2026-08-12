//go:build integration

package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	adapterlocal "github.com/kenyamaneko/overload-party-scenario/internal/adapter/local"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/story"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// contractPlayerID は結合テストで一貫して使う認証コンテキストの playerID。
// scenario.player_onboarding / scenario.player_story_progress の player_id が UUID 型のため TST- 接頭辞は使えない。
const contractPlayerID = "00000000-0000-4000-8000-000000000001"

func init() {
	gin.SetMode(gin.TestMode)
}

// newStoryTestEngine は StoryHandler を internalauth 経由でマウントした gin.Engine を組み立てる。
func newStoryTestEngine(h *StoryHandler) *gin.Engine {
	verifier := &internalauth.MockVerifier{VerifyFn: func(string) (string, error) { return contractPlayerID, nil }}
	r := gin.New()
	r.Use(gin.Recovery())
	api := r.Group("/api/v1/scenarios")
	api.Use(internalauth.VerifyInternalAuth(verifier))
	api.GET("/episodes", h.ListEpisodes)
	api.GET("/episodes/:episodeId/script", h.GetScript)
	api.POST("/episodes/:episodeId/complete", h.CompleteEpisode)
	return r
}

// newStoryHandlerForTest は実 postgres リポジトリ・ローカルファイル ScriptStore・account フェイクで StoryHandler を組み立てる。
func newStoryHandlerForTest(scriptRoot, accountURL string) *StoryHandler {
	account := adapterhttp.NewAccountClient(accountURL)
	return NewStoryHandler(story.New(
		postgres.NewStoryRepository(sharedPg.Pool),
		adapterlocal.NewScriptStore(scriptRoot),
		account,
	))
}

// doAuthedRequest は X-Internal-Auth 付きリクエストを engine に対して発行する。
func doAuthedRequest(t *testing.T, engine *gin.Engine, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(internalauth.HeaderName, "TST.any.token")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

// mustJSON は v を JSON エンコードする (エンコード失敗時は即座にテストを失敗させる)。
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// decodeError はレスポンスボディの {"error": "..."} から error 文字列を取り出す。
func decodeError(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Error
}

// writeScriptFile は root からの相対 key にスクリプト本文のファイルを書き込む。
func writeScriptFile(t *testing.T, root, key, body string) {
	t.Helper()
	full := filepath.Join(root, key)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
}

// writeScriptAsDir は root からの相対 key にディレクトリを作り、ScriptStore の未検出以外のエラー (インフラエラー) を再現する。
func writeScriptAsDir(t *testing.T, root, key string) {
	t.Helper()
	full := filepath.Join(root, key)
	require.NoError(t, os.MkdirAll(full, 0o755))
}

// countStoryProgress は指定プレイヤー・エピソードの scenario.player_story_progress 行数を返す。
func countStoryProgress(t *testing.T, playerID, episodeID string) int {
	t.Helper()
	var n int
	require.NoError(t, sharedPg.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM scenario.player_story_progress WHERE player_id = $1 AND episode_id = $2`,
		playerID, episodeID,
	).Scan(&n))
	return n
}

func TestStoryHandler(t *testing.T) {
	t.Run("[シナリオ]エピソード一覧取得", func(t *testing.T) {
		t.Run("認証済みプレイヤーがアクティブなエピソード一覧を要求すると、200でアンロック状態付きの一覧が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", 1, "stories/TST-EP-0001/{lang}.ks", true)
			seedEpisode(t, "TST-EP-0002", 99, "stories/TST-EP-0002/{lang}.ks", true)
			accountSrv := newAccountFake(t, 10)
			h := newStoryHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newStoryTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodGet, "/api/v1/scenarios/episodes", nil)

			require.Equal(t, http.StatusOK, w.Code)
			var body apiscenario.EpisodesListResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Len(t, body.Episodes, 2)
			unlocked := map[string]bool{}
			for _, ep := range body.Episodes {
				unlocked[ep.EpisodeID] = ep.IsUnlocked
			}
			require.Contains(t, unlocked, "TST-EP-0001")
			require.Contains(t, unlocked, "TST-EP-0002")
			assert.True(t, unlocked["TST-EP-0001"])
			assert.False(t, unlocked["TST-EP-0002"])
		})

		t.Run("langクエリパラメータが省略されたとき、日本語のタイトルで一覧が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", 1, "stories/TST-EP-0001/{lang}.ks", true)
			accountSrv := newAccountFake(t, 10)
			h := newStoryHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newStoryTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodGet, "/api/v1/scenarios/episodes", nil)

			require.Equal(t, http.StatusOK, w.Code)
			var body apiscenario.EpisodesListResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Len(t, body.Episodes, 1)
			assert.Equal(t, "タイトル", body.Episodes[0].Title)
		})

		t.Run("エピソード一覧取得の内部でエラーが起きたとき、500が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			accountSrv := newAccountFake(t, 10)
			accountSrv.GetPlayerFn = func() (int, any) { return http.StatusInternalServerError, nil }
			h := newStoryHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newStoryTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodGet, "/api/v1/scenarios/episodes", nil)

			assert.Equal(t, http.StatusInternalServerError, w.Code)
		})
	})

	t.Run("[シナリオ]スクリプト取得", func(t *testing.T) {
		t.Run("アンロック済みエピソードのスクリプトを要求すると、200でスクリプト本文が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", 1, "stories/TST-EP-0001/{lang}.ks", true)
			root := t.TempDir()
			writeScriptFile(t, root, "stories/TST-EP-0001/ja.ks", "TST story script body")
			accountSrv := newAccountFake(t, 10)
			h := newStoryHandlerForTest(root, accountSrv.URL())
			engine := newStoryTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodGet, "/api/v1/scenarios/episodes/TST-EP-0001/script", nil)

			require.Equal(t, http.StatusOK, w.Code)
			var body apiscenario.ScenarioScriptResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, "TST-EP-0001", body.EpisodeID)
			assert.Equal(t, "TST story script body", body.Script)
		})

		t.Run("存在しないエピソードIDを要求すると、404が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			accountSrv := newAccountFake(t, 10)
			h := newStoryHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newStoryTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodGet, "/api/v1/scenarios/episodes/TST-EP-9999/script", nil)

			assert.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("非アクティブなエピソードを要求すると、404が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", 1, "stories/TST-EP-0001/{lang}.ks", false)
			accountSrv := newAccountFake(t, 10)
			h := newStoryHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newStoryTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodGet, "/api/v1/scenarios/episodes/TST-EP-0001/script", nil)

			assert.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("アンロック済みかつアクティブなエピソードのスクリプトファイルがストアに存在しないとき、404が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", 1, "stories/TST-EP-0001/{lang}.ks", true)
			accountSrv := newAccountFake(t, 10)
			h := newStoryHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newStoryTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodGet, "/api/v1/scenarios/episodes/TST-EP-0001/script", nil)

			assert.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("ロック済みエピソードを要求すると、403が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", 99, "stories/TST-EP-0001/{lang}.ks", true)
			accountSrv := newAccountFake(t, 1)
			h := newStoryHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newStoryTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodGet, "/api/v1/scenarios/episodes/TST-EP-0001/script", nil)

			assert.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("スクリプトストアが未検出以外のエラー(インフラエラー)を返すとき、500が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", 1, "stories/TST-EP-0001/{lang}.ks", true)
			root := t.TempDir()
			writeScriptAsDir(t, root, "stories/TST-EP-0001/ja.ks")
			accountSrv := newAccountFake(t, 10)
			h := newStoryHandlerForTest(root, accountSrv.URL())
			engine := newStoryTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodGet, "/api/v1/scenarios/episodes/TST-EP-0001/script", nil)

			assert.Equal(t, http.StatusInternalServerError, w.Code)
		})
	})

	t.Run("[シナリオ]エピソード完了記録", func(t *testing.T) {
		t.Run("アンロック済みエピソードの完了を要求すると、200で完了メッセージが返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", 1, "stories/TST-EP-0001/{lang}.ks", true)
			accountSrv := newAccountFake(t, 10)
			h := newStoryHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newStoryTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodPost, "/api/v1/scenarios/episodes/TST-EP-0001/complete", nil)

			require.Equal(t, http.StatusOK, w.Code)
			var body apiscenario.ScenarioCompleteResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, "TST-EP-0001", body.EpisodeID)
			assert.Equal(t, "episode completed", body.Message)
			assert.Equal(t, 1, countStoryProgress(t, contractPlayerID, "TST-EP-0001"))
		})

		t.Run("存在しないエピソードIDを要求すると、404が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			accountSrv := newAccountFake(t, 10)
			h := newStoryHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newStoryTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodPost, "/api/v1/scenarios/episodes/TST-EP-9999/complete", nil)

			assert.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("非アクティブなエピソードの完了を要求すると、404が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", 1, "stories/TST-EP-0001/{lang}.ks", false)
			accountSrv := newAccountFake(t, 10)
			h := newStoryHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newStoryTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodPost, "/api/v1/scenarios/episodes/TST-EP-0001/complete", nil)

			assert.Equal(t, http.StatusNotFound, w.Code)
			assert.Equal(t, 0, countStoryProgress(t, contractPlayerID, "TST-EP-0001"))
		})

		t.Run("ロック済みエピソードの完了を要求すると、403が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", 99, "stories/TST-EP-0001/{lang}.ks", true)
			accountSrv := newAccountFake(t, 1)
			h := newStoryHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newStoryTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodPost, "/api/v1/scenarios/episodes/TST-EP-0001/complete", nil)

			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.Equal(t, 0, countStoryProgress(t, contractPlayerID, "TST-EP-0001"))
		})
	})
}
