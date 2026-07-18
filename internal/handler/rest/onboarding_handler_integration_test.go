//go:build integration

package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-scenario/internal/adapter/local"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/onboarding"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// stubNameValidator は account 表示名バリデーション (外部境界) を固定結果に差し替える。
type stubNameValidator struct{ err error }

// ValidateOnboardingName は account バリデーションの成否を注入値で制御し、handler の分岐を引き起こす。
func (s stubNameValidator) ValidateOnboardingName(context.Context, string) error { return s.err }

// stubPlayerReader は account からの初期 faction 取得 (外部境界) を固定結果に差し替える。
type stubPlayerReader struct {
	player port.AccountPlayer
	err    error
}

// GetOnboardingPlayer は account からの player 取得結果を注入値で再現し、Complete の分岐を引き起こす。
func (s stubPlayerReader) GetOnboardingPlayer(context.Context) (port.AccountPlayer, error) {
	return s.player, s.err
}

// toStringPtr は値型のテーブルケースから AccountPlayer.InitialFaction (ポインタ型) を組むための変換ヘルパ。
func toStringPtr(s string) *string { return &s }

// newOnboardingEngine はオンボーディング 4 エンドポイントを実 PostgreSQL の OnboardingRepo で組んだエンジンを返す。
func newOnboardingEngine(playerID, scriptRoot string, nameValidator port.OnboardingNameValidator, playerReader port.OnboardingPlayerReader) *gin.Engine {
	svc := onboarding.New(
		postgres.NewOnboardingRepository(sharedPg.Pool),
		local.NewScriptStore(scriptRoot),
		nameValidator, playerReader,
	)
	h := NewOnboardingHandler(svc)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(internalauth.PlayerIDContextKey, playerID) })
	r.GET("/onboarding/script", h.GetScript)
	r.PUT("/onboarding/name", h.UpdateName)
	r.POST("/onboarding/faction", h.SelectFaction)
	r.POST("/onboarding/complete", h.Complete)
	return r
}

// seedOnboardingComplete は MarkComplete を経由せず完了済みの player_onboarding 行を直接作る。
func seedOnboardingComplete(t *testing.T, playerID string) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.player_onboarding (player_id) VALUES ($1)`, playerID)
	require.NoError(t, err)
}

// countOnboarding は Complete の永続化結果を公開エントリ越しに観測するため player_onboarding の行数を読む。
func countOnboarding(t *testing.T, playerID string) int {
	t.Helper()
	var n int
	require.NoError(t, sharedPg.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM scenario.player_onboarding WHERE player_id = $1`, playerID).Scan(&n))
	return n
}

// countOutbox は publish 系エンドポイントの outbox 反映を観測するため outbox_events の行数を読む。
func countOutbox(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, sharedPg.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM scenario.outbox_events`).Scan(&n))
	return n
}

// fetchLatestOutboxEvent は outbox_events の最新 1 行の event_type と payload を読む。
func fetchLatestOutboxEvent(t *testing.T) (string, []byte) {
	t.Helper()
	var eventType string
	var payload []byte
	require.NoError(t, sharedPg.Pool.QueryRow(context.Background(),
		`SELECT event_type, payload FROM scenario.outbox_events ORDER BY created_at DESC LIMIT 1`,
	).Scan(&eventType, &payload))
	return eventType, payload
}

func TestOnboardingGetScriptContract(t *testing.T) {
	t.Run("オンボーディングスクリプト取得の応答契約", func(t *testing.T) {
		tests := []struct {
			name       string
			setup      func(t *testing.T, scriptRoot string)
			wantStatus int
			wantBody   string
		}{
			{
				name:       "スクリプトが無いとき、404 になる",
				setup:      func(t *testing.T, _ string) {},
				wantStatus: http.StatusNotFound,
				wantBody:   "script not found",
			},
			{
				name: "スクリプトが在るとき、200 で本文を返す",
				setup: func(t *testing.T, scriptRoot string) {
					writeScript(t, scriptRoot, "scripts/onboarding/ja.ks", "プロローグ本文")
				},
				wantStatus: http.StatusOK,
				wantBody:   "プロローグ本文",
			},
			{
				name: "スクリプトの保存先が読み取れない状態のとき、500 になる",
				setup: func(t *testing.T, scriptRoot string) {
					require.NoError(t, os.MkdirAll(filepath.Join(scriptRoot, "scripts/onboarding/ja.ks"), 0o755))
				},
				wantStatus: http.StatusInternalServerError,
				wantBody:   "story script infrastructure error",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				scriptRoot := t.TempDir()
				tt.setup(t, scriptRoot)

				req := httptest.NewRequest(http.MethodGet, "/onboarding/script?lang=ja", nil)
				w := httptest.NewRecorder()
				newOnboardingEngine(contractPlayerID, scriptRoot, stubNameValidator{}, stubPlayerReader{}).ServeHTTP(w, req)

				assert.Equal(t, tt.wantStatus, w.Code)
				assert.Contains(t, w.Body.String(), tt.wantBody)
			})
		}

		t.Run("lang を指定しないとき、日本語のスクリプトが返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			scriptRoot := t.TempDir()
			writeScript(t, scriptRoot, "scripts/onboarding/ja.ks", "日本語プロローグ")
			writeScript(t, scriptRoot, "scripts/onboarding/en.ks", "english prologue")

			req := httptest.NewRequest(http.MethodGet, "/onboarding/script", nil)
			w := httptest.NewRecorder()
			newOnboardingEngine(contractPlayerID, scriptRoot, stubNameValidator{}, stubPlayerReader{}).ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), "日本語プロローグ")
		})
	})
}

func TestOnboardingUpdateNameContract(t *testing.T) {
	t.Run("表示名更新の応答契約", func(t *testing.T) {
		tests := []struct {
			name       string
			validator  stubNameValidator
			body       string
			wantStatus int
			wantBody   string
			wantOutbox int
		}{
			{
				name:       "account が表示名を拒否するとき、400 になる",
				validator:  stubNameValidator{err: port.ErrInvalidName},
				body:       `{"name":"x"}`,
				wantStatus: http.StatusBadRequest,
				wantBody:   "invalid name",
				wantOutbox: 0,
			},
			{
				name:       "account に player が無いとき、404 になる",
				validator:  stubNameValidator{err: port.ErrPlayerNotFound},
				body:       `{"name":"Kenya"}`,
				wantStatus: http.StatusNotFound,
				wantBody:   "player not found",
				wantOutbox: 0,
			},
			{
				name:       "JSON が壊れているとき、400 になる",
				validator:  stubNameValidator{},
				body:       `{"name":`,
				wantStatus: http.StatusBadRequest,
				wantOutbox: 0,
			},
			{
				name:       "正常系のとき、204 で onboarding-name-set を outbox へ積む",
				validator:  stubNameValidator{},
				body:       `{"name":"Kenya"}`,
				wantStatus: http.StatusNoContent,
				wantOutbox: 1,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)

				req := httptest.NewRequest(http.MethodPut, "/onboarding/name", strings.NewReader(tt.body))
				w := httptest.NewRecorder()
				newOnboardingEngine(contractPlayerID, t.TempDir(), tt.validator, stubPlayerReader{}).ServeHTTP(w, req)

				assert.Equal(t, tt.wantStatus, w.Code)
				assert.Contains(t, w.Body.String(), tt.wantBody)
				assert.Equal(t, tt.wantOutbox, countOutbox(t))
			})
		}
	})
}

func TestOnboardingSelectFactionContract(t *testing.T) {
	t.Run("初期 faction 選択の応答契約", func(t *testing.T) {
		tests := []struct {
			name       string
			body       string
			wantStatus int
			wantBody   string
			wantOutbox int
		}{
			{
				name:       "SelectableFactions 外のとき、400 になる",
				body:       `{"initial_faction_id":"Neutral"}`,
				wantStatus: http.StatusBadRequest,
				wantBody:   "invalid initial faction",
				wantOutbox: 0,
			},
			{
				name:       "JSON が壊れているとき、400 になる",
				body:       `{"initial_faction_id":`,
				wantStatus: http.StatusBadRequest,
				wantOutbox: 0,
			},
			{
				name:       "正常系のとき、204 で onboarding-faction-set を outbox へ積む",
				body:       `{"initial_faction_id":"SHE"}`,
				wantStatus: http.StatusNoContent,
				wantOutbox: 1,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)

				req := httptest.NewRequest(http.MethodPost, "/onboarding/faction", strings.NewReader(tt.body))
				w := httptest.NewRecorder()
				newOnboardingEngine(contractPlayerID, t.TempDir(), stubNameValidator{}, stubPlayerReader{}).ServeHTTP(w, req)

				assert.Equal(t, tt.wantStatus, w.Code)
				assert.Contains(t, w.Body.String(), tt.wantBody)
				assert.Equal(t, tt.wantOutbox, countOutbox(t))
			})
		}
	})
}

func TestOnboardingSelectFactionOutboxPayload(t *testing.T) {
	t.Run("選択可能な全 faction での初期陣営選択", func(t *testing.T) {
		tests := []struct {
			name      string
			factionID string
		}{
			{name: "Tenki を選ぶと、204 で outbox の payload の faction が Tenki になる", factionID: "Tenki"},
			{name: "Sugar を選ぶと、204 で outbox の payload の faction が Sugar になる", factionID: "Sugar"},
			{name: "Tuners を選ぶと、204 で outbox の payload の faction が Tuners になる", factionID: "Tuners"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)

				body := fmt.Sprintf(`{"initial_faction_id":%q}`, tt.factionID)
				req := httptest.NewRequest(http.MethodPost, "/onboarding/faction", strings.NewReader(body))
				w := httptest.NewRecorder()
				newOnboardingEngine(contractPlayerID, t.TempDir(), stubNameValidator{}, stubPlayerReader{}).ServeHTTP(w, req)

				require.Equal(t, http.StatusNoContent, w.Code)
				eventType, payload := fetchLatestOutboxEvent(t)
				assert.Equal(t, apiscenario.EventTypeOnboardingFactionSet, eventType)
				var decoded apiscenario.OnboardingFactionSetEvent
				require.NoError(t, json.Unmarshal(payload, &decoded))
				assert.Equal(t, tt.factionID, decoded.InitialFactionID)
			})
		}
	})
}

func TestOnboardingCompleteContract(t *testing.T) {
	t.Run("オンボーディング完了の応答契約", func(t *testing.T) {
		tests := []struct {
			name          string
			setup         func(t *testing.T)
			reader        stubPlayerReader
			wantStatus    int
			wantBody      string
			wantOnboarded int
		}{
			{
				name:          "account に player が無いとき、404 になる",
				setup:         func(t *testing.T) {},
				reader:        stubPlayerReader{err: port.ErrPlayerNotFound},
				wantStatus:    http.StatusNotFound,
				wantBody:      "player not found",
				wantOnboarded: 0,
			},
			{
				name:          "初期 faction 未選択のとき、409 になる",
				setup:         func(t *testing.T) {},
				reader:        stubPlayerReader{player: port.AccountPlayer{InitialFaction: nil}},
				wantStatus:    http.StatusConflict,
				wantBody:      "initial faction not selected",
				wantOnboarded: 0,
			},
			{
				name: "二度目の完了のとき、409 になる",
				setup: func(t *testing.T) {
					seedOnboardingComplete(t, contractPlayerID)
				},
				reader:        stubPlayerReader{player: port.AccountPlayer{InitialFaction: toStringPtr("SHE")}},
				wantStatus:    http.StatusConflict,
				wantBody:      "already completed",
				wantOnboarded: 1,
			},
			{
				name:          "account の予期せぬ障害のとき、500 になる",
				setup:         func(t *testing.T) {},
				reader:        stubPlayerReader{err: errors.New("account unavailable")},
				wantStatus:    http.StatusInternalServerError,
				wantBody:      "account unavailable",
				wantOnboarded: 0,
			},
			{
				name:          "正常系のとき、200 で player_onboarding を記録する",
				setup:         func(t *testing.T) {},
				reader:        stubPlayerReader{player: port.AccountPlayer{InitialFaction: toStringPtr("SHE")}},
				wantStatus:    http.StatusOK,
				wantBody:      "onboarding completed",
				wantOnboarded: 1,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sharedPg.Truncate(t)
				tt.setup(t)

				req := httptest.NewRequest(http.MethodPost, "/onboarding/complete", nil)
				w := httptest.NewRecorder()
				newOnboardingEngine(contractPlayerID, t.TempDir(), stubNameValidator{}, tt.reader).ServeHTTP(w, req)

				assert.Equal(t, tt.wantStatus, w.Code)
				assert.Contains(t, w.Body.String(), tt.wantBody)
				assert.Equal(t, tt.wantOnboarded, countOnboarding(t, contractPlayerID))
			})
		}
	})
}
