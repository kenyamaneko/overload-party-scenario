// Package http は account が所有するデータを読み書きするための REST クライアントを提供する。
//
// Why: 他リポからの import を構造的に禁じ、account への呼び出し経路を scenario 内に閉じ込めるため。
package http

import (
	"context"
	"errors"
	"fmt"
	nethttp "net/http"

	apiaccount "github.com/kenyamaneko/overload-party-account/packages/api-account"
	"github.com/kenyamaneko/overload-party-account/packages/api-account/apiaccountclient"
	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

// AccountClient は scenario が account に対して同期書込・読み取りを行う唯一の経路。
type AccountClient struct {
	api *apiaccountclient.Client
}

// NewAccountClient は AccountClient を生成する。
func NewAccountClient(baseURL string) *AccountClient {
	api, err := apiaccountclient.New(baseURL,
		apiaccountclient.WithRequestEditorFn(func(ctx context.Context, req *nethttp.Request) error {
			internalauth.InjectHeader(ctx, req.Header)
			return nil
		}),
	)
	if err != nil {
		panic(fmt.Sprintf("accountclient: %v", err))
	}
	return &AccountClient{api: api}
}

// ValidateOnboardingName は account へ表示名のバリデーションを同期で問い合わせる。
func (c *AccountClient) ValidateOnboardingName(ctx context.Context, name string) error {
	err := c.api.ValidateNameForOnboarding(ctx, apiaccount.ValidateNameForOnboardingRequest{Name: name})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, apiaccountclient.ErrBadRequest):
		return fmt.Errorf("%w: %v", port.ErrInvalidName, err)
	case errors.Is(err, apiaccountclient.ErrNotFound):
		return port.ErrPlayerNotFound
	}
	return err
}

// GetOnboardingPlayer は account の players レコードから initial_faction を取得する。
func (c *AccountClient) GetOnboardingPlayer(ctx context.Context) (port.AccountPlayer, error) {
	resp, err := c.api.GetPlayer(ctx)
	if errors.Is(err, apiaccountclient.ErrNotFound) {
		return port.AccountPlayer{}, port.ErrPlayerNotFound
	}
	if err != nil {
		return port.AccountPlayer{}, err
	}
	return port.AccountPlayer{
		PlayerID:       resp.PlayerID,
		InitialFaction: resp.InitialFaction,
	}, nil
}

// GetPlayerProgress はアンロック判定に使う level と保有 faction を account から取得する。
func (c *AccountClient) GetPlayerProgress(ctx context.Context) (port.PlayerProgress, error) {
	player, err := c.api.GetPlayer(ctx)
	if errors.Is(err, apiaccountclient.ErrNotFound) {
		return port.PlayerProgress{}, port.ErrPlayerNotFound
	}
	if err != nil {
		return port.PlayerProgress{}, err
	}

	factions, err := c.api.ListFactions(ctx)
	if errors.Is(err, apiaccountclient.ErrNotFound) {
		return port.PlayerProgress{}, port.ErrPlayerNotFound
	}
	if err != nil {
		return port.PlayerProgress{}, err
	}

	return port.PlayerProgress{
		Level:         player.Level,
		OwnedFactions: factions.Factions,
	}, nil
}
