package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiaccount "github.com/kenyamaneko/overload-party-account/packages/api-account"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

// newStubAccountServer は指定した status と body を全リクエストに返す account スタブサーバを起動する。
func newStubAccountServer(t *testing.T, status int, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAccountClient_ValidateOnboardingName(t *testing.T) {
	t.Run("表示名バリデーションのHTTP応答翻訳", func(t *testing.T) {
		tests := []struct {
			name   string
			status int
			verify func(t *testing.T, err error)
		}{
			{
				name:   "accountが表示名を400で拒否するとき、ErrInvalidNameになる",
				status: http.StatusBadRequest,
				verify: func(t *testing.T, err error) {
					require.Error(t, err)
					assert.ErrorIs(t, err, port.ErrInvalidName)
				},
			},
			{
				name:   "accountに対象プレイヤーが無く404のとき、ErrPlayerNotFoundになる",
				status: http.StatusNotFound,
				verify: func(t *testing.T, err error) {
					require.Error(t, err)
					assert.ErrorIs(t, err, port.ErrPlayerNotFound)
				},
			},
			{
				name:   "accountが表示名を受理するとき、エラーにならない",
				status: http.StatusNoContent,
				verify: func(t *testing.T, err error) {
					assert.NoError(t, err)
				},
			},
			{
				name:   "accountが500を返すとき、表示名検証はエラーになる",
				status: http.StatusInternalServerError,
				verify: func(t *testing.T, err error) {
					assert.Error(t, err)
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				srv := newStubAccountServer(t, tt.status, nil)
				c := NewAccountClient(srv.URL)

				err := c.ValidateOnboardingName(context.Background(), "Kenya")
				tt.verify(t, err)
			})
		}
	})
}

func TestAccountClient_GetOnboardingPlayer(t *testing.T) {
	t.Run("オンボード用プレイヤー取得のHTTP応答翻訳", func(t *testing.T) {
		t.Run("accountがプレイヤー情報を返すとき、プレイヤーIDと初期陣営が取得できる", func(t *testing.T) {
			faction := "SHE"
			resp := apiaccount.PlayerResponse{
				PlayerID:         "TST-P1",
				FirebaseUID:      "tst-firebase-uid",
				InitialFaction:   &faction,
				OnboardingStatus: apiaccount.OnboardingStatus("completed"),
				CreatedAt:        time.Now().UTC(),
				UpdatedAt:        time.Now().UTC(),
			}
			body, err := json.Marshal(resp)
			require.NoError(t, err)
			srv := newStubAccountServer(t, http.StatusOK, body)
			c := NewAccountClient(srv.URL)

			player, err := c.GetOnboardingPlayer(context.Background())
			require.NoError(t, err)
			assert.Equal(t, "TST-P1", player.PlayerID)
			require.NotNil(t, player.InitialFaction)
			assert.Equal(t, "SHE", *player.InitialFaction)
		})

		t.Run("accountにプレイヤーが無く404のとき、ErrPlayerNotFoundになる", func(t *testing.T) {
			srv := newStubAccountServer(t, http.StatusNotFound, nil)
			c := NewAccountClient(srv.URL)

			_, err := c.GetOnboardingPlayer(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, port.ErrPlayerNotFound)
		})
	})
}
