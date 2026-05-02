// Package http はオンボーディング限定の account REST クライアントを提供する。
// scenario の onboarding ユースケース内に閉じた利用に限定し、汎用 account
// クライアントとして他ユースケース・他サービスから再利用しない方針を構造的に
//表すため、packages/ ではなく internal/adapter/ 配下に置く (ARCHITECTURE.md
// 「account 直叩きの構造的封じ込め」)。
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	nethttp "net/http"
	"net/url"
	"time"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

// defaultTimeout は account への 1 リクエスト全体のタイムアウト。
// onboarding は同期 UX のため長時間ブロックは避けたい。account 側 handler の
// 想定処理時間 (DB 1 hop) より十分長く、UI のローディング許容範囲より短い 5 秒に
// 揃える。
const defaultTimeout = 5 * time.Second

// AccountClient は scenario onboarding が account に対して同期書込・読み取りを
// 行う唯一の経路。port を狭く保つため、onboarding 用途以外の account
// エンドポイントには触らない。
type AccountClient struct {
	baseURL string
	http    *nethttp.Client
}

// NewAccountClient は AccountClient を生成する。baseURL は ClusterIP DNS
// (例: http://account.<ns>.svc.cluster.local:9000) を想定し、env 経由で注入する。
func NewAccountClient(baseURL string) *AccountClient {
	return &AccountClient{
		baseURL: baseURL,
		http:    &nethttp.Client{Timeout: defaultTimeout},
	}
}

type validateNameRequest struct {
	Name string `json:"name"`
}

// ValidateOnboardingName は account の POST
// /internal/v1/players/:playerId/onboarding/name/validate を呼び、表示名のバリデーションのみを
// 同期で確認する。account 側で書き込みは行わない (書き込みは scenario が後段で
// onboarding-name-set event を publish し、account subscriber が同一 tx で永続化する)。
//
// account の 400 (ErrInvalidName 相当) は port.ErrInvalidName に、404 は
// port.ErrPlayerNotFound に翻訳して usecase 層へ伝える。
func (c *AccountClient) ValidateOnboardingName(ctx context.Context, playerID, name string) error {
	if playerID == "" {
		return errors.New("accountclient: playerID is empty")
	}
	path := "/internal/v1/players/" + url.PathEscape(playerID) + "/onboarding/name/validate"
	body, err := json.Marshal(validateNameRequest{Name: name})
	if err != nil {
		return fmt.Errorf("accountclient: marshal: %w", err)
	}
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("accountclient: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("accountclient: do: %w", err)
	}
	defer resp.Body.Close()

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

// playerResponse は account の GET /internal/v1/players/:playerId が返す
// PlayerResponse の JSON 形を、scenario が使う最小フィールドのみで写したもの。
// 余分なフィールド (level / exp / name 等) は意図的に取り込まず、Complete 時に
// 必要な属性以外を scenario 内に持ち込まないことを構造的に保証する。
type playerResponse struct {
	PlayerID        string  `json:"player_id"`
	SelectedFaction *string `json:"selected_faction,omitempty"`
}

// GetOnboardingPlayer は account の GET /internal/v1/players/:playerId を呼び、
// 完了 publish 時の InitialFactionID 用に selected_faction を取得する。
// selected_faction が nil の場合は faction 選択ステップを経ずに Complete API が
// 叩かれたフロー違反として port.ErrFactionNotSelected を返す。
func (c *AccountClient) GetOnboardingPlayer(ctx context.Context, playerID string) (port.AccountPlayer, error) {
	if playerID == "" {
		return port.AccountPlayer{}, errors.New("accountclient: playerID is empty")
	}
	path := "/internal/v1/players/" + url.PathEscape(playerID)
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return port.AccountPlayer{}, fmt.Errorf("accountclient: new request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return port.AccountPlayer{}, fmt.Errorf("accountclient: do: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case nethttp.StatusOK:
		var out playerResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return port.AccountPlayer{}, fmt.Errorf("accountclient: decode: %w", err)
		}
		return port.AccountPlayer{
			PlayerID:        out.PlayerID,
			SelectedFaction: out.SelectedFaction,
		}, nil
	case nethttp.StatusNotFound:
		return port.AccountPlayer{}, port.ErrPlayerNotFound
	}
	raw, _ := io.ReadAll(resp.Body)
	return port.AccountPlayer{}, fmt.Errorf("accountclient: GET %s: status %d: %s", path, resp.StatusCode, string(raw))
}
