package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	"github.com/kenyamaneko/overload-party-scenario/internal/handler/rest"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/onboarding"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/story"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeRouterVerifier は router 単体テスト用の internalauth.Verifier 最小 fake。
type fakeRouterVerifier struct {
	playerID string
	err      error
}

// Verify は固定の playerID / err を返す。
func (f fakeRouterVerifier) Verify(string) (string, error) {
	return f.playerID, f.err
}

// stubStoryRepo はエピソードなしを返す port.StoryRepo スタブ。
type stubStoryRepo struct{}

// ListActiveEpisodes はエピソードなしを返す。
func (stubStoryRepo) ListActiveEpisodes(context.Context) ([]*domain.Episode, error) {
	return []*domain.Episode{}, nil
}

// FindEpisodeByID はエピソード不在を返す。
func (stubStoryRepo) FindEpisodeByID(context.Context, string) (*domain.Episode, error) {
	return nil, port.ErrNotFound
}

// GetCompletedEpisodeIDs は完了なしを返す。
func (stubStoryRepo) GetCompletedEpisodeIDs(context.Context, string) ([]string, error) {
	return []string{}, nil
}

// GetUnlockContext は空のアンロック状態を返す。
func (stubStoryRepo) GetUnlockContext(context.Context, string) (*domain.UnlockContext, error) {
	return &domain.UnlockContext{}, nil
}

// MarkComplete は常に成功する。
func (stubStoryRepo) MarkComplete(context.Context, string, string) error {
	return nil
}

// stubScriptStore は固定のスクリプト本文を返す port.ScriptStore スタブ。
type stubScriptStore struct{}

// ReadScript は固定本文を返す。
func (stubScriptStore) ReadScript(context.Context, string) (string, error) {
	return "TST script", nil
}

// stubOnboardingRepo は常に成功する port.OnboardingRepo スタブ。
type stubOnboardingRepo struct{}

// MarkComplete は常に成功する。
func (stubOnboardingRepo) MarkComplete(context.Context, string, ...port.OutboxEvent) error {
	return nil
}

// PublishEvents は常に成功する。
func (stubOnboardingRepo) PublishEvents(context.Context, ...port.OutboxEvent) error {
	return nil
}

// stubNameValidator は常に妥当と判定する port.OnboardingNameValidator スタブ。
type stubNameValidator struct{}

// ValidateOnboardingName は常に成功する。
func (stubNameValidator) ValidateOnboardingName(context.Context, string) error {
	return nil
}

// stubPlayerReader は zero value のプレイヤーを返す port.OnboardingPlayerReader スタブ。
type stubPlayerReader struct{}

// GetOnboardingPlayer は zero value のプレイヤーを返す。
func (stubPlayerReader) GetOnboardingPlayer(context.Context) (port.AccountPlayer, error) {
	return port.AccountPlayer{}, nil
}

// newTestRouter はスタブ port で組んだ実 usecase / handler で router を構築する。
func newTestRouter(verifier internalauth.Verifier) *gin.Engine {
	storyS := story.New(stubStoryRepo{}, stubScriptStore{})
	onboardingS := onboarding.New(stubOnboardingRepo{}, stubScriptStore{}, stubNameValidator{}, stubPlayerReader{})
	return New(rest.NewStoryHandler(storyS), rest.NewOnboardingHandler(onboardingS), verifier)
}

// TestNew_HealthEndpoint は /health が auth middleware を通らず 200 を返すことを確かめる。
func TestNew_HealthEndpoint(t *testing.T) {
	r := newTestRouter(fakeRouterVerifier{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestNew_ApiRouteRequiresInternalAuth は /api/v1/scenarios 配下の全ルートが
// auth header 欠落で 401 を返し handler に到達しないことを確かめる。
func TestNew_ApiRouteRequiresInternalAuth(t *testing.T) {
	r := newTestRouter(fakeRouterVerifier{playerID: "TST-PLAYER-1"})

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "GET /episodes は auth header 欠落で 401",
			method: http.MethodGet,
			path:   "/api/v1/scenarios/episodes",
		},
		{
			name:   "GET /episodes/:episodeId/script は auth header 欠落で 401",
			method: http.MethodGet,
			path:   "/api/v1/scenarios/episodes/EP-TST-0001/script",
		},
		{
			name:   "POST /episodes/:episodeId/complete は auth header 欠落で 401",
			method: http.MethodPost,
			path:   "/api/v1/scenarios/episodes/EP-TST-0001/complete",
		},
		{
			name:   "GET /onboarding/script は auth header 欠落で 401",
			method: http.MethodGet,
			path:   "/api/v1/scenarios/onboarding/script",
		},
		{
			name:   "PUT /onboarding/name は auth header 欠落で 401",
			method: http.MethodPut,
			path:   "/api/v1/scenarios/onboarding/name",
		},
		{
			name:   "POST /onboarding/faction は auth header 欠落で 401",
			method: http.MethodPost,
			path:   "/api/v1/scenarios/onboarding/faction",
		},
		{
			name:   "POST /onboarding/complete は auth header 欠落で 401",
			method: http.MethodPost,
			path:   "/api/v1/scenarios/onboarding/complete",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

// TestNew_ApiRouteRejectsVerifierError は verifier が error を返すと 401 を返し
// handler に到達しないことを確かめる。
func TestNew_ApiRouteRejectsVerifierError(t *testing.T) {
	r := newTestRouter(fakeRouterVerifier{err: errors.New("invalid token")})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenarios/episodes", nil)
	req.Header.Set(internalauth.HeaderName, "any.token")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestNew_ApiRouteWithValidTokenReachesHandler は verifier を通過したリクエストが
// handler の成功応答まで到達することを確かめる。
func TestNew_ApiRouteWithValidTokenReachesHandler(t *testing.T) {
	r := newTestRouter(fakeRouterVerifier{playerID: "TST-PLAYER-1"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenarios/episodes", nil)
	req.Header.Set(internalauth.HeaderName, "any.token")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
