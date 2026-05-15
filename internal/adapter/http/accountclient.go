// Package http はオンボーディング限定の account REST クライアントを提供する。
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

// AccountClient は scenario onboarding が account に対して同期書込・読み取りを行う唯一の経路。
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
