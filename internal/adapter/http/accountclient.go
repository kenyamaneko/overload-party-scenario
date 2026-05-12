// Package http はオンボーディング限定の account REST クライアントを提供する。
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"time"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

// defaultTimeout は account への 1 リクエスト全体のタイムアウト。
const defaultTimeout = 5 * time.Second

// AccountClient は scenario onboarding が account に対して同期書込・読み取りを行う唯一の経路。
type AccountClient struct {
	baseURL string
	http    *nethttp.Client
}

// NewAccountClient は AccountClient を生成する。
func NewAccountClient(baseURL string) *AccountClient {
	return &AccountClient{
		baseURL: baseURL,
		http:    &nethttp.Client{Timeout: defaultTimeout},
	}
}

type validateNameRequest struct {
	Name string `json:"name"`
}

// ValidateOnboardingName は account へ表示名のバリデーションを同期で問い合わせる。
// player_id は ctx に乗った X-Internal-Auth JWT の sub から account 側で解決する。
func (c *AccountClient) ValidateOnboardingName(ctx context.Context, name string) error {
	const path = "/api/v1/account/me/onboarding/name/validate"
	body, err := json.Marshal(validateNameRequest{Name: name})
	if err != nil {
		return fmt.Errorf("accountclient: marshal: %w", err)
	}
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("accountclient: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	internalauth.InjectHeader(ctx, req.Header)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("accountclient: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case nethttp.StatusOK, nethttp.StatusNoContent:
		return nil
	case nethttp.StatusBadRequest:
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: %s", port.ErrInvalidName, string(raw))
	case nethttp.StatusNotFound:
		return port.ErrPlayerNotFound
	}
	raw, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("accountclient: POST %s: status %d: %s", path, resp.StatusCode, string(raw))
}

// playerResponse は account の GET /api/v1/account/me レスポンスを、scenario が使う最小フィールドだけ写したもの。
type playerResponse struct {
	PlayerID       string  `json:"player_id"`
	InitialFaction *string `json:"initial_faction,omitempty"`
}

// GetOnboardingPlayer は account の players レコードから initial_faction を取得する。
// player_id は ctx に乗った X-Internal-Auth JWT の sub から account 側で解決する。
func (c *AccountClient) GetOnboardingPlayer(ctx context.Context) (port.AccountPlayer, error) {
	const path = "/api/v1/account/me"
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return port.AccountPlayer{}, fmt.Errorf("accountclient: new request: %w", err)
	}
	internalauth.InjectHeader(ctx, req.Header)

	resp, err := c.http.Do(req)
	if err != nil {
		return port.AccountPlayer{}, fmt.Errorf("accountclient: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case nethttp.StatusOK:
		var out playerResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return port.AccountPlayer{}, fmt.Errorf("accountclient: decode: %w", err)
		}
		return port.AccountPlayer{
			PlayerID:       out.PlayerID,
			InitialFaction: out.InitialFaction,
		}, nil
	case nethttp.StatusNotFound:
		return port.AccountPlayer{}, port.ErrPlayerNotFound
	}
	raw, _ := io.ReadAll(resp.Body)
	return port.AccountPlayer{}, fmt.Errorf("accountclient: GET %s: status %d: %s", path, resp.StatusCode, string(raw))
}

