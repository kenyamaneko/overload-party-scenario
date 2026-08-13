//go:build integration

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiaccount "github.com/kenyamaneko/overload-party-account/packages/api-account"
	gamedesign "github.com/kenyamaneko/overload-party-common/packages/game-design-constants"
	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"

	adapterhttp "github.com/kenyamaneko/overload-party-scenario/internal/adapter/http"
	adapterlocal "github.com/kenyamaneko/overload-party-scenario/internal/adapter/local"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/onboarding"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// newOnboardingTestEngine は OnboardingHandler を internalauth 経由でマウントした gin.Engine を組み立てる。
func newOnboardingTestEngine(h *OnboardingHandler) *gin.Engine {
	verifier := &internalauth.MockVerifier{VerifyFn: func(string) (string, error) { return contractPlayerID, nil }}
	r := gin.New()
	r.Use(gin.Recovery())
	api := r.Group("/api/v1/scenarios")
	api.Use(internalauth.VerifyInternalAuth(verifier))
	api.GET("/onboarding/script", h.GetScript)
	api.PUT("/onboarding/name", h.UpdateName)
	api.POST("/onboarding/faction", h.SelectFaction)
	api.POST("/onboarding/complete", h.Complete)
	return r
}

// newOnboardingHandlerForTest は実 postgres リポジトリ・ローカルファイル ScriptStore・account フェイクで OnboardingHandler を組み立てる。
func newOnboardingHandlerForTest(scriptRoot, accountURL string) *OnboardingHandler {
	account := adapterhttp.NewAccountClient(accountURL)
	return NewOnboardingHandler(onboarding.New(
		postgres.NewOnboardingRepository(sharedPg.Pool),
		adapterlocal.NewScriptStore(scriptRoot),
		account,
		account,
	))
}

// seedOnboarded は scenario.player_onboarding にオンボード完了済み行を作る。
func seedOnboarded(t *testing.T, playerID string) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.player_onboarding (player_id) VALUES ($1)`, playerID)
	require.NoError(t, err)
}

// isPlayerOnboarded は指定プレイヤーの scenario.player_onboarding 行が存在するかを返す。
func isPlayerOnboarded(t *testing.T, playerID string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, sharedPg.Pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM scenario.player_onboarding WHERE player_id = $1)`,
		playerID,
	).Scan(&exists))
	return exists
}

// countOutboxEventsByType は指定 event_type の scenario.outbox_events 行数を返す。
func countOutboxEventsByType(t *testing.T, eventType string) int {
	t.Helper()
	var n int
	require.NoError(t, sharedPg.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM scenario.outbox_events WHERE event_type = $1`, eventType,
	).Scan(&n))
	return n
}

func TestOnboardingHandler(t *testing.T) {
	t.Run("[オンボーディング]スクリプト取得", func(t *testing.T) {
		t.Run("スクリプトが存在するとき、200で本文が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			root := t.TempDir()
			writeScriptFile(t, root, "scripts/onboarding/ja.ks", "TST onboarding script body")
			accountSrv := newAccountFake(t, 0)
			h := newOnboardingHandlerForTest(root, accountSrv.URL())
			engine := newOnboardingTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodGet, "/api/v1/scenarios/onboarding/script", nil)

			require.Equal(t, http.StatusOK, w.Code)
			var body apiscenario.OnboardingScriptResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, "TST onboarding script body", body.Script)
		})

		t.Run("スクリプトが存在しないとき、404が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			accountSrv := newAccountFake(t, 0)
			h := newOnboardingHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newOnboardingTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodGet, "/api/v1/scenarios/onboarding/script", nil)

			assert.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("スクリプトストアが未検出以外のエラーを返すとき、500が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			root := t.TempDir()
			writeScriptAsDir(t, root, "scripts/onboarding/ja.ks")
			accountSrv := newAccountFake(t, 0)
			h := newOnboardingHandlerForTest(root, accountSrv.URL())
			engine := newOnboardingTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodGet, "/api/v1/scenarios/onboarding/script", nil)

			assert.Equal(t, http.StatusInternalServerError, w.Code)
		})
	})

	t.Run("[オンボーディング]名前入力ステップ", func(t *testing.T) {
		t.Run("有効な名前を送ると、204が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			accountSrv := newAccountFake(t, 0)
			h := newOnboardingHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newOnboardingTestEngine(h)

			reqBody := mustJSON(t, apiscenario.OnboardingNameRequest{Name: "TST Name"})
			w := doAuthedRequest(t, engine, http.MethodPut, "/api/v1/scenarios/onboarding/name", reqBody)

			require.Equal(t, http.StatusNoContent, w.Code)
			assert.Equal(t, 1, countOutboxEventsByType(t, "onboarding_name_set"))
		})

		t.Run("不正なJSONボディを送ると、400が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			accountSrv := newAccountFake(t, 0)
			h := newOnboardingHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newOnboardingTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodPut, "/api/v1/scenarios/onboarding/name", []byte(`{"name":`))

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.NotEqual(t, "onboarding: invalid name", decodeError(t, w))
		})

		t.Run("account側で無効と判定される名前を送ると、400が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			accountSrv := newAccountFake(t, 0)
			accountSrv.ValidateNameForOnboardingFn = func(apiaccount.ValidateNameForOnboardingRequest) (int, any) {
				return http.StatusBadRequest, nil
			}
			h := newOnboardingHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newOnboardingTestEngine(h)

			reqBody := mustJSON(t, apiscenario.OnboardingNameRequest{Name: "TST Invalid Name"})
			w := doAuthedRequest(t, engine, http.MethodPut, "/api/v1/scenarios/onboarding/name", reqBody)

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, "onboarding: invalid name", decodeError(t, w))
		})

		t.Run("対象プレイヤーがaccountに存在しないとき、404が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			accountSrv := newAccountFake(t, 0)
			accountSrv.ValidateNameForOnboardingFn = func(apiaccount.ValidateNameForOnboardingRequest) (int, any) {
				return http.StatusNotFound, nil
			}
			h := newOnboardingHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newOnboardingTestEngine(h)

			reqBody := mustJSON(t, apiscenario.OnboardingNameRequest{Name: "TST Name"})
			w := doAuthedRequest(t, engine, http.MethodPut, "/api/v1/scenarios/onboarding/name", reqBody)

			require.Equal(t, http.StatusNotFound, w.Code)
			assert.Equal(t, "onboarding: player not found", decodeError(t, w))
		})
	})

	t.Run("[オンボーディング]陣営選択ステップ", func(t *testing.T) {
		t.Run("選択可能な陣営を送ると、204が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			accountSrv := newAccountFake(t, 0)
			h := newOnboardingHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newOnboardingTestEngine(h)

			reqBody := mustJSON(t, apiscenario.OnboardingFactionRequest{InitialFactionID: gamedesign.SelectableFactions[0]})
			w := doAuthedRequest(t, engine, http.MethodPost, "/api/v1/scenarios/onboarding/faction", reqBody)

			require.Equal(t, http.StatusNoContent, w.Code)
			assert.Equal(t, 1, countOutboxEventsByType(t, "onboarding_faction_set"))
		})

		t.Run("不正なJSONボディを送ると、400が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			accountSrv := newAccountFake(t, 0)
			h := newOnboardingHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newOnboardingTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodPost, "/api/v1/scenarios/onboarding/faction", []byte(`{"initial_faction_id":`))

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.NotEqual(t, "onboarding: invalid initial faction", decodeError(t, w))
		})

		t.Run("選択可能でない陣営を送ると、400が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			accountSrv := newAccountFake(t, 0)
			h := newOnboardingHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newOnboardingTestEngine(h)

			reqBody := mustJSON(t, apiscenario.OnboardingFactionRequest{InitialFactionID: gamedesign.FactionNeutral})
			w := doAuthedRequest(t, engine, http.MethodPost, "/api/v1/scenarios/onboarding/faction", reqBody)

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, "onboarding: invalid initial faction", decodeError(t, w))
		})
	})

	t.Run("[オンボーディング]オンボーディング完了", func(t *testing.T) {
		t.Run("陣営選択済みの未完了プレイヤーが完了を要求すると、200で完了メッセージとplayerIDが返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			accountSrv := newAccountFake(t, 0)
			faction := gamedesign.SelectableFactions[0]
			accountSrv.GetPlayerFn = func() (int, any) {
				return http.StatusOK, apiaccount.PlayerResponse{PlayerID: contractPlayerID, InitialFaction: &faction}
			}
			h := newOnboardingHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newOnboardingTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodPost, "/api/v1/scenarios/onboarding/complete", nil)

			require.Equal(t, http.StatusOK, w.Code)
			var body apiscenario.OnboardingCompleteResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, "onboarding completed", body.Message)
			assert.Equal(t, contractPlayerID, body.PlayerID)
			assert.True(t, isPlayerOnboarded(t, contractPlayerID))
			assert.Equal(t, 1, countOutboxEventsByType(t, "player_onboarded"))
		})

		t.Run("陣営未選択のプレイヤーが完了を要求すると、409が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			accountSrv := newAccountFake(t, 0)
			accountSrv.GetPlayerFn = func() (int, any) {
				return http.StatusOK, apiaccount.PlayerResponse{PlayerID: contractPlayerID, InitialFaction: nil}
			}
			h := newOnboardingHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newOnboardingTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodPost, "/api/v1/scenarios/onboarding/complete", nil)

			require.Equal(t, http.StatusConflict, w.Code)
			assert.Equal(t, "onboarding: initial faction not selected", decodeError(t, w))
		})

		t.Run("既にオンボード完了済みのプレイヤーが完了を要求すると、409が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedOnboarded(t, contractPlayerID)
			accountSrv := newAccountFake(t, 0)
			faction := gamedesign.SelectableFactions[0]
			accountSrv.GetPlayerFn = func() (int, any) {
				return http.StatusOK, apiaccount.PlayerResponse{PlayerID: contractPlayerID, InitialFaction: &faction}
			}
			h := newOnboardingHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newOnboardingTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodPost, "/api/v1/scenarios/onboarding/complete", nil)

			require.Equal(t, http.StatusConflict, w.Code)
			assert.Equal(t, "onboarding: already completed", decodeError(t, w))
		})

		t.Run("対象プレイヤーがaccountに存在しないとき、404が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			accountSrv := newAccountFake(t, 0)
			accountSrv.GetPlayerFn = func() (int, any) { return http.StatusNotFound, nil }
			h := newOnboardingHandlerForTest(t.TempDir(), accountSrv.URL())
			engine := newOnboardingTestEngine(h)

			w := doAuthedRequest(t, engine, http.MethodPost, "/api/v1/scenarios/onboarding/complete", nil)

			require.Equal(t, http.StatusNotFound, w.Code)
			assert.Equal(t, "onboarding: player not found", decodeError(t, w))
		})
	})
}
