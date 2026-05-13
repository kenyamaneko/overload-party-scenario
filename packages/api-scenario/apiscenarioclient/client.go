// Package apiscenarioclient は consumer から wire 詳細 (REST path / HTTP status code) を
// 隠蔽するため、生成 client を sentinel error 変換でラップする。
package apiscenarioclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// Sentinel errors. wire 契約 (OpenAPI spec) で定義された status code の意味を sentinel として export する。
var (
	// ErrNotFound は status 404。
	ErrNotFound = errors.New("apiscenarioclient: not found")

	// ErrUnauthorized は status 401。
	ErrUnauthorized = errors.New("apiscenarioclient: unauthorized")

	// ErrForbidden は status 403。getEpisodeScript / completeEpisode で前提エピソード未達 等。
	ErrForbidden = errors.New("apiscenarioclient: forbidden")

	// ErrBadRequest は status 400。
	ErrBadRequest = errors.New("apiscenarioclient: bad request")

	// ErrConflict は status 409。getOnboardingScript / completeOnboarding で onboarding 状態が想定と異なる場合。
	ErrConflict = errors.New("apiscenarioclient: conflict")

	// ErrInternalServer は status 5xx。
	ErrInternalServer = errors.New("apiscenarioclient: internal server error")
)

// Client は overload-party-scenario REST API の sentinel-error converted client。
type Client struct {
	api *apiscenario.ClientWithResponses
}

// Option は Client 構築時の設定。
type Option func(*config)

type config struct {
	httpClient apiscenario.HttpRequestDoer
	editors    []apiscenario.RequestEditorFn
}

// WithHTTPClient は基底 HTTP doer を差し替える。デフォルトは http.DefaultClient。
func WithHTTPClient(doer apiscenario.HttpRequestDoer) Option {
	return func(c *config) { c.httpClient = doer }
}

// WithRequestEditorFn は全リクエストに適用する RequestEditor を追加する。
func WithRequestEditorFn(fn apiscenario.RequestEditorFn) Option {
	return func(c *config) { c.editors = append(c.editors, fn) }
}

// New は baseURL に接続する Client を生成する。
func New(baseURL string, opts ...Option) (*Client, error) {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	apiOpts := make([]apiscenario.ClientOption, 0, 1+len(cfg.editors))
	if cfg.httpClient != nil {
		apiOpts = append(apiOpts, apiscenario.WithHTTPClient(cfg.httpClient))
	}
	for _, ed := range cfg.editors {
		apiOpts = append(apiOpts, apiscenario.WithRequestEditorFn(ed))
	}

	api, err := apiscenario.NewClientWithResponses(baseURL, apiOpts...)
	if err != nil {
		return nil, fmt.Errorf("apiscenarioclient: new: %w", err)
	}
	return &Client{api: api}, nil
}

// GetHealth はサービス死活監視用エンドポイントを叩く。
func (c *Client) GetHealth(ctx context.Context) (*apiscenario.HealthResponse, error) {
	resp, err := c.api.GetHealthWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("apiscenarioclient: GetHealth: %w", err)
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, statusError("GetHealth", resp.StatusCode())
}

// ListEpisodes は指定言語で公開エピソード一覧を返す。
func (c *Client) ListEpisodes(ctx context.Context, lang string) (*apiscenario.EpisodesListResponse, error) {
	resp, err := c.api.ListEpisodesWithResponse(ctx, &apiscenario.ListEpisodesParams{Lang: &lang})
	if err != nil {
		return nil, fmt.Errorf("apiscenarioclient: ListEpisodes: %w", err)
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, statusError("ListEpisodes", resp.StatusCode())
}

// GetEpisodeScript は指定エピソードのスクリプトを返す。
func (c *Client) GetEpisodeScript(ctx context.Context, episodeID string, lang string) (*apiscenario.ScenarioScriptResponse, error) {
	resp, err := c.api.GetEpisodeScriptWithResponse(ctx, episodeID, &apiscenario.GetEpisodeScriptParams{Lang: &lang})
	if err != nil {
		return nil, fmt.Errorf("apiscenarioclient: GetEpisodeScript: %w", err)
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, statusError("GetEpisodeScript", resp.StatusCode())
}

// CompleteEpisode はエピソード完了を記録する。
func (c *Client) CompleteEpisode(ctx context.Context, episodeID string) (*apiscenario.ScenarioCompleteResponse, error) {
	resp, err := c.api.CompleteEpisodeWithResponse(ctx, episodeID)
	if err != nil {
		return nil, fmt.Errorf("apiscenarioclient: CompleteEpisode: %w", err)
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, statusError("CompleteEpisode", resp.StatusCode())
}

// GetOnboardingScript はオンボーディングスクリプトを返す。
func (c *Client) GetOnboardingScript(ctx context.Context, lang string) (*apiscenario.OnboardingScriptResponse, error) {
	resp, err := c.api.GetOnboardingScriptWithResponse(ctx, &apiscenario.GetOnboardingScriptParams{Lang: &lang})
	if err != nil {
		return nil, fmt.Errorf("apiscenarioclient: GetOnboardingScript: %w", err)
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, statusError("GetOnboardingScript", resp.StatusCode())
}

// UpdateOnboardingName はオンボーディング表示名を更新する。spec は 204 を返すため戻り値は error のみ。
func (c *Client) UpdateOnboardingName(ctx context.Context, req apiscenario.OnboardingNameRequest) error {
	resp, err := c.api.UpdateOnboardingNameWithResponse(ctx, apiscenario.UpdateOnboardingNameJSONRequestBody(req))
	if err != nil {
		return fmt.Errorf("apiscenarioclient: UpdateOnboardingName: %w", err)
	}
	if resp.StatusCode() == http.StatusNoContent {
		return nil
	}
	return statusError("UpdateOnboardingName", resp.StatusCode())
}

// SelectOnboardingFaction はオンボーディングで初期 faction を選択する。spec は 204 を返すため戻り値は error のみ。
func (c *Client) SelectOnboardingFaction(ctx context.Context, req apiscenario.OnboardingFactionRequest) error {
	resp, err := c.api.SelectOnboardingFactionWithResponse(ctx, apiscenario.SelectOnboardingFactionJSONRequestBody(req))
	if err != nil {
		return fmt.Errorf("apiscenarioclient: SelectOnboardingFaction: %w", err)
	}
	if resp.StatusCode() == http.StatusNoContent {
		return nil
	}
	return statusError("SelectOnboardingFaction", resp.StatusCode())
}

// CompleteOnboarding はオンボーディング完了を記録する。
func (c *Client) CompleteOnboarding(ctx context.Context) (*apiscenario.OnboardingCompleteResponse, error) {
	resp, err := c.api.CompleteOnboardingWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("apiscenarioclient: CompleteOnboarding: %w", err)
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, statusError("CompleteOnboarding", resp.StatusCode())
}

// statusError は HTTP status code を sentinel error (errors.Is 分岐可能) に変換する。
func statusError(op string, code int) error {
	var sentinel error
	switch {
	case code == http.StatusUnauthorized:
		sentinel = ErrUnauthorized
	case code == http.StatusForbidden:
		sentinel = ErrForbidden
	case code == http.StatusNotFound:
		sentinel = ErrNotFound
	case code == http.StatusBadRequest:
		sentinel = ErrBadRequest
	case code == http.StatusConflict:
		sentinel = ErrConflict
	case code >= http.StatusInternalServerError:
		sentinel = ErrInternalServer
	default:
		return fmt.Errorf("apiscenarioclient: %s: unexpected status %d", op, code)
	}
	return fmt.Errorf("apiscenarioclient: %s: %w", op, sentinel)
}
