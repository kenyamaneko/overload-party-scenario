//go:build integration

package http

import (
	"context"
	nethttp "net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiaccount "github.com/kenyamaneko/overload-party-account/packages/api-account"
	"github.com/kenyamaneko/overload-party-account/packages/api-account/apiaccountclient"
	"github.com/kenyamaneko/overload-party-account/packages/api-account/apiaccountserverfake"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

func newTestAccountServer(t *testing.T) *apiaccountserverfake.Server {
	t.Helper()
	srv := apiaccountserverfake.NewServer()
	t.Cleanup(srv.Close)
	return srv
}

func TestAccountClient_ValidateOnboardingName(t *testing.T) {
	t.Run("[オンボーディング]表示名バリデーション", func(t *testing.T) {
		t.Run("accountが成功を返すとき、エラーなく完了する", func(t *testing.T) {
			srv := newTestAccountServer(t)
			var gotReq apiaccount.ValidateNameForOnboardingRequest
			srv.ValidateNameForOnboardingFn = func(req apiaccount.ValidateNameForOnboardingRequest) (int, any) {
				gotReq = req
				return nethttp.StatusNoContent, nil
			}
			c := NewAccountClient(srv.URL())

			err := c.ValidateOnboardingName(context.Background(), "Kenya")

			assert.NoError(t, err)
			assert.Equal(t, "Kenya", gotReq.Name)
		})

		cases := []struct {
			name      string
			status    int
			wantErrIs error
		}{
			{
				name:      "accountが不正リクエストを返すとき、無効な名前を表すエラーになる",
				status:    nethttp.StatusBadRequest,
				wantErrIs: port.ErrInvalidName,
			},
			{
				name:      "accountが未検出を返すとき、プレイヤー未検出を表すエラーになる",
				status:    nethttp.StatusNotFound,
				wantErrIs: port.ErrPlayerNotFound,
			},
			{
				name:      "accountがその他のエラーを返すとき、そのエラーがそのまま返る",
				status:    nethttp.StatusInternalServerError,
				wantErrIs: apiaccountclient.ErrInternalServer,
			},
		}
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				srv := newTestAccountServer(t)
				srv.ValidateNameForOnboardingFn = func(apiaccount.ValidateNameForOnboardingRequest) (int, any) {
					return tt.status, nil
				}
				c := NewAccountClient(srv.URL())

				err := c.ValidateOnboardingName(context.Background(), "Kenya")

				assert.ErrorIs(t, err, tt.wantErrIs)
			})
		}
	})
}

func TestAccountClient_GetOnboardingPlayer(t *testing.T) {
	t.Run("[オンボーディング]オンボード用プレイヤー取得", func(t *testing.T) {
		faction := "TST-FACTION-1"
		cases := []struct {
			name               string
			player             apiaccount.PlayerResponse
			wantPlayerID       string
			wantInitialFaction *string
		}{
			{
				name: "accountが初期陣営設定済みのプレイヤー情報を返すとき、その陣営IDを含むプレイヤー情報が返る",
				player: apiaccount.PlayerResponse{
					PlayerID:       "TST-PLAYER-1",
					InitialFaction: &faction,
				},
				wantPlayerID:       "TST-PLAYER-1",
				wantInitialFaction: &faction,
			},
			{
				name: "accountが初期陣営未設定のプレイヤー情報を返すとき、初期陣営が未設定のプレイヤー情報が返る",
				player: apiaccount.PlayerResponse{
					PlayerID:       "TST-PLAYER-2",
					InitialFaction: nil,
				},
				wantPlayerID:       "TST-PLAYER-2",
				wantInitialFaction: nil,
			},
		}
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				srv := newTestAccountServer(t)
				srv.GetPlayerFn = func() (int, any) {
					return nethttp.StatusOK, tt.player
				}
				c := NewAccountClient(srv.URL())

				got, err := c.GetOnboardingPlayer(context.Background())

				require.NoError(t, err)
				assert.Equal(t, tt.wantPlayerID, got.PlayerID)
				assert.Equal(t, tt.wantInitialFaction, got.InitialFaction)
			})
		}

		errCases := []struct {
			name      string
			status    int
			wantErrIs error
		}{
			{
				name:      "accountが未検出を返すとき、プレイヤー未検出を表すエラーになる",
				status:    nethttp.StatusNotFound,
				wantErrIs: port.ErrPlayerNotFound,
			},
			{
				name:      "accountがその他のエラーを返すとき、そのエラーがそのまま返る",
				status:    nethttp.StatusInternalServerError,
				wantErrIs: apiaccountclient.ErrInternalServer,
			},
		}
		for _, tt := range errCases {
			t.Run(tt.name, func(t *testing.T) {
				srv := newTestAccountServer(t)
				srv.GetPlayerFn = func() (int, any) {
					return tt.status, nil
				}
				c := NewAccountClient(srv.URL())

				_, err := c.GetOnboardingPlayer(context.Background())

				assert.ErrorIs(t, err, tt.wantErrIs)
			})
		}
	})
}

func TestAccountClient_GetPlayerProgress(t *testing.T) {
	t.Run("[シナリオ]到達状況取得", func(t *testing.T) {
		t.Run("accountがプレイヤー情報と陣営一覧をともに返すとき、レベルと所持陣営を含む到達状況が返る", func(t *testing.T) {
			srv := newTestAccountServer(t)
			srv.GetPlayerFn = func() (int, any) {
				return nethttp.StatusOK, apiaccount.PlayerResponse{PlayerID: "TST-PLAYER-1", Level: 7}
			}
			srv.ListFactionsFn = func() (int, any) {
				return nethttp.StatusOK, apiaccount.FactionListing{Factions: []string{"TST-FACTION-1", "TST-FACTION-2"}}
			}
			c := NewAccountClient(srv.URL())

			got, err := c.GetPlayerProgress(context.Background())

			require.NoError(t, err)
			assert.Equal(t, int64(7), got.Level)
			assert.Equal(t, []string{"TST-FACTION-1", "TST-FACTION-2"}, got.OwnedFactions)
		})

		cases := []struct {
			name               string
			getPlayerStatus    int
			listFactionsStatus int
			wantErrIs          error
		}{
			{
				name:               "プレイヤー情報取得で未検出が返るとき、プレイヤー未検出を表すエラーになる",
				getPlayerStatus:    nethttp.StatusNotFound,
				listFactionsStatus: nethttp.StatusOK,
				wantErrIs:          port.ErrPlayerNotFound,
			},
			{
				name:               "プレイヤー情報取得がその他のエラーを返すとき、そのエラーがそのまま返る",
				getPlayerStatus:    nethttp.StatusInternalServerError,
				listFactionsStatus: nethttp.StatusOK,
				wantErrIs:          apiaccountclient.ErrInternalServer,
			},
			{
				name:               "陣営一覧取得で未検出が返るとき、プレイヤー未検出を表すエラーになる",
				getPlayerStatus:    nethttp.StatusOK,
				listFactionsStatus: nethttp.StatusNotFound,
				wantErrIs:          port.ErrPlayerNotFound,
			},
			{
				name:               "陣営一覧取得がその他のエラーを返すとき、そのエラーがそのまま返る",
				getPlayerStatus:    nethttp.StatusOK,
				listFactionsStatus: nethttp.StatusInternalServerError,
				wantErrIs:          apiaccountclient.ErrInternalServer,
			},
		}
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				srv := newTestAccountServer(t)
				srv.GetPlayerFn = func() (int, any) {
					return tt.getPlayerStatus, apiaccount.PlayerResponse{PlayerID: "TST-PLAYER-1"}
				}
				srv.ListFactionsFn = func() (int, any) {
					return tt.listFactionsStatus, apiaccount.FactionListing{Factions: []string{}}
				}
				c := NewAccountClient(srv.URL())

				_, err := c.GetPlayerProgress(context.Background())

				assert.ErrorIs(t, err, tt.wantErrIs)
			})
		}
	})
}
