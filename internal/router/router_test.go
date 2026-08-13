package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gamedesign "github.com/kenyamaneko/overload-party-common/packages/game-design-constants"
	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	"github.com/kenyamaneko/overload-party-scenario/internal/handler/rest"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	"github.com/kenyamaneko/overload-party-scenario/internal/router"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/onboarding"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/story"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

const (
	testPlayerID          = "TST-PLAYER-1"
	testEpisodeID         = "TST-0001"
	testStoryScriptBody   = "TST story script body"
	testOnboardScriptBody = "TST onboarding script body"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type stubStoryRepo struct{}

func (stubStoryRepo) ListActiveEpisodes(context.Context) ([]*domain.Episode, error) {
	return []*domain.Episode{}, nil
}

func (stubStoryRepo) FindEpisodeByID(context.Context, string) (*domain.Episode, error) {
	return &domain.Episode{
		EpisodeID:  testEpisodeID,
		IsActive:   true,
		ScriptPath: "scripts/story/" + testEpisodeID + "/{lang}.ks",
	}, nil
}

func (stubStoryRepo) GetCompletedEpisodeIDs(context.Context, string) ([]string, error) {
	return []string{}, nil
}

func (stubStoryRepo) MarkComplete(context.Context, string, string) error {
	return nil
}

type stubPlayerProgressReader struct{}

func (stubPlayerProgressReader) GetPlayerProgress(context.Context) (port.PlayerProgress, error) {
	return port.PlayerProgress{}, nil
}

type stubScriptStore struct {
	body string
}

func (s stubScriptStore) ReadScript(context.Context, string) (string, error) {
	return s.body, nil
}

type stubOnboardingRepo struct{}

func (stubOnboardingRepo) MarkComplete(context.Context, string, ...port.OutboxEvent) error {
	return nil
}

func (stubOnboardingRepo) PublishEvents(context.Context, ...port.OutboxEvent) error {
	return nil
}

// stubNameValidator は空文字の name のみ拒否する。UpdateName と SelectFaction の
// 取り違え (どちらも成功応答になり得る) を、name 未設定時の 400 で検出可能にする。
type stubNameValidator struct{}

func (stubNameValidator) ValidateOnboardingName(_ context.Context, name string) error {
	if name == "" {
		return port.ErrInvalidName
	}
	return nil
}

type stubPlayerReader struct{}

func (stubPlayerReader) GetOnboardingPlayer(context.Context) (port.AccountPlayer, error) {
	faction := gamedesign.SelectableFactions[0]
	return port.AccountPlayer{PlayerID: testPlayerID, InitialFaction: &faction}, nil
}

func newTestRouter(verifier internalauth.Verifier) *gin.Engine {
	storyS := story.New(stubStoryRepo{}, stubScriptStore{body: testStoryScriptBody}, stubPlayerProgressReader{})
	onboardingS := onboarding.New(stubOnboardingRepo{}, stubScriptStore{body: testOnboardScriptBody}, stubNameValidator{}, stubPlayerReader{})
	return router.New(rest.NewStoryHandler(storyS), rest.NewOnboardingHandler(onboardingS), verifier)
}

func authenticatedVerifier() *internalauth.MockVerifier {
	return &internalauth.MockVerifier{
		VerifyFn: func(string) (string, error) { return testPlayerID, nil },
	}
}

func TestNew(t *testing.T) {
	t.Run("[ルーティング]/health", func(t *testing.T) {
		t.Run("認証なしでアクセスすると、200が返る", func(t *testing.T) {
			// VerifyFn 未設定: /health が verifier に到達しないことの検出を兼ねる
			r := newTestRouter(&internalauth.MockVerifier{})

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

			assert.Equal(t, http.StatusOK, w.Code)
		})
	})

	t.Run("[ルーティング]認証が必須なエンドポイントへのアクセス", func(t *testing.T) {
		cases := []struct {
			name   string
			method string
			path   string
		}{
			{name: "GET /api/v1/scenarios/episodesは、認証ヘッダが無いと401が返る", method: http.MethodGet, path: "/api/v1/scenarios/episodes"},
			{name: "GET /api/v1/scenarios/episodes/:episodeId/scriptは、認証ヘッダが無いと401が返る", method: http.MethodGet, path: "/api/v1/scenarios/episodes/" + testEpisodeID + "/script"},
			{name: "POST /api/v1/scenarios/episodes/:episodeId/completeは、認証ヘッダが無いと401が返る", method: http.MethodPost, path: "/api/v1/scenarios/episodes/" + testEpisodeID + "/complete"},
			{name: "GET /api/v1/scenarios/onboarding/scriptは、認証ヘッダが無いと401が返る", method: http.MethodGet, path: "/api/v1/scenarios/onboarding/script"},
			{name: "PUT /api/v1/scenarios/onboarding/nameは、認証ヘッダが無いと401が返る", method: http.MethodPut, path: "/api/v1/scenarios/onboarding/name"},
			{name: "POST /api/v1/scenarios/onboarding/factionは、認証ヘッダが無いと401が返る", method: http.MethodPost, path: "/api/v1/scenarios/onboarding/faction"},
			{name: "POST /api/v1/scenarios/onboarding/completeは、認証ヘッダが無いと401が返る", method: http.MethodPost, path: "/api/v1/scenarios/onboarding/complete"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				// VerifyFn 未設定: 認証ヘッダ欠落時は middleware が verifier に到達しないことの検出を兼ねる
				r := newTestRouter(&internalauth.MockVerifier{})

				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))

				assert.Equal(t, http.StatusUnauthorized, w.Code)
			})
		}

		t.Run("GET /api/v1/scenarios/episodesは、認証トークンが無効だと401が返る", func(t *testing.T) {
			r := newTestRouter(&internalauth.MockVerifier{
				VerifyFn: func(string) (string, error) { return "", errors.New("invalid token") },
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/scenarios/episodes", nil)
			req.Header.Set(internalauth.HeaderName, "invalid.token")
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	})

	t.Run("[ルーティング]認証済みリクエストのルーティング", func(t *testing.T) {
		t.Run("GET /api/v1/scenarios/episodesは、エピソード一覧が返る", func(t *testing.T) {
			r := newTestRouter(authenticatedVerifier())

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/scenarios/episodes", nil)
			req.Header.Set(internalauth.HeaderName, "any.token")
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			var body apiscenario.EpisodesListResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Len(t, body.Episodes, 0)
		})

		t.Run("GET /api/v1/scenarios/episodes/:episodeId/scriptは、該当エピソードのスクリプトが返る", func(t *testing.T) {
			r := newTestRouter(authenticatedVerifier())

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/scenarios/episodes/"+testEpisodeID+"/script", nil)
			req.Header.Set(internalauth.HeaderName, "any.token")
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			var body apiscenario.ScenarioScriptResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, testEpisodeID, body.EpisodeID)
			assert.Equal(t, testStoryScriptBody, body.Script)
		})

		t.Run("POST /api/v1/scenarios/episodes/:episodeId/completeは、エピソード完了の応答が返る", func(t *testing.T) {
			r := newTestRouter(authenticatedVerifier())

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/scenarios/episodes/"+testEpisodeID+"/complete", nil)
			req.Header.Set(internalauth.HeaderName, "any.token")
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			var body apiscenario.ScenarioCompleteResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, testEpisodeID, body.EpisodeID)
			assert.Equal(t, "episode completed", body.Message)
		})

		t.Run("GET /api/v1/scenarios/onboarding/scriptは、オンボーディングスクリプトが返る", func(t *testing.T) {
			r := newTestRouter(authenticatedVerifier())

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/scenarios/onboarding/script", nil)
			req.Header.Set(internalauth.HeaderName, "any.token")
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			var body apiscenario.OnboardingScriptResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, testOnboardScriptBody, body.Script)
		})

		t.Run("PUT /api/v1/scenarios/onboarding/nameは、204が返る", func(t *testing.T) {
			r := newTestRouter(authenticatedVerifier())

			reqBody, err := json.Marshal(apiscenario.OnboardingNameRequest{Name: "TST Name"})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/scenarios/onboarding/name", bytes.NewReader(reqBody))
			req.Header.Set(internalauth.HeaderName, "any.token")
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusNoContent, w.Code)
			assert.Empty(t, w.Body.Bytes())
		})

		t.Run("POST /api/v1/scenarios/onboarding/factionは、204が返る", func(t *testing.T) {
			r := newTestRouter(authenticatedVerifier())

			reqBody, err := json.Marshal(apiscenario.OnboardingFactionRequest{InitialFactionID: gamedesign.SelectableFactions[0]})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/scenarios/onboarding/faction", bytes.NewReader(reqBody))
			req.Header.Set(internalauth.HeaderName, "any.token")
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusNoContent, w.Code)
			assert.Empty(t, w.Body.Bytes())
		})

		t.Run("POST /api/v1/scenarios/onboarding/completeは、オンボーディング完了の応答が返る", func(t *testing.T) {
			r := newTestRouter(authenticatedVerifier())

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/scenarios/onboarding/complete", nil)
			req.Header.Set(internalauth.HeaderName, "any.token")
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			var body apiscenario.OnboardingCompleteResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, "onboarding completed", body.Message)
			assert.Equal(t, testPlayerID, body.PlayerID)
		})
	})
}
