package rest

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/onboarding"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// OnboardingHandler はオンボーディング (初回プロローグ) の REST ハンドラを提供する。
type OnboardingHandler struct {
	svc *onboarding.Service
}

// NewOnboardingHandler は OnboardingHandler を構築する。
func NewOnboardingHandler(svc *onboarding.Service) *OnboardingHandler {
	return &OnboardingHandler{svc: svc}
}

// GetScript はオンボーディングシナリオ本文を返す。
func (h *OnboardingHandler) GetScript(c *gin.Context) {
	playerID := c.GetString(internalauth.PlayerIDContextKey)
	lang := c.DefaultQuery("lang", "ja")

	body, err := h.svc.GetScript(c.Request.Context(), playerID, lang)
	if err != nil {
		logGetOnboardingScriptError(err, playerID, lang)
		respondOnboardingError(c, err)
		return
	}

	c.JSON(http.StatusOK, apiscenario.OnboardingScriptResponse{Script: body})
}

// UpdateName はオンボード内 name 入力ステップを処理する。
func (h *OnboardingHandler) UpdateName(c *gin.Context) {
	playerID := c.GetString(internalauth.PlayerIDContextKey)

	var req apiscenario.OnboardingNameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Info("update onboarding name bind failed", "error", err, "player_id", playerID)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.UpdateName(c.Request.Context(), playerID, req.Name); err != nil {
		if isOnboardingRejected(err) {
			slog.Info("update onboarding name rejected", "error", err, "player_id", playerID)
		} else {
			slog.Error("update onboarding name failed", "error", err, "player_id", playerID)
		}
		respondOnboardingError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// SelectFaction はオンボード内 faction 選択ステップを処理する。
func (h *OnboardingHandler) SelectFaction(c *gin.Context) {
	playerID := c.GetString(internalauth.PlayerIDContextKey)

	var req apiscenario.OnboardingFactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Info("select onboarding faction bind failed", "error", err, "player_id", playerID)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.SelectFaction(c.Request.Context(), playerID, req.InitialFactionID); err != nil {
		if isOnboardingRejected(err) {
			slog.Info("select onboarding faction rejected", "error", err, "player_id", playerID)
		} else {
			slog.Error("select onboarding faction failed", "error", err, "player_id", playerID)
		}
		respondOnboardingError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// Complete はオンボーディング完了を記録し、player-onboarded を outbox に atomic に積む。
func (h *OnboardingHandler) Complete(c *gin.Context) {
	playerID := c.GetString(internalauth.PlayerIDContextKey)

	if err := h.svc.Complete(c.Request.Context(), playerID); err != nil {
		if isOnboardingRejected(err) {
			slog.Info("complete onboarding rejected", "error", err, "player_id", playerID)
		} else {
			slog.Error("complete onboarding failed", "error", err, "player_id", playerID)
		}
		respondOnboardingError(c, err)
		return
	}

	c.JSON(http.StatusOK, apiscenario.OnboardingCompleteResponse{
		Message:  "onboarding completed",
		PlayerID: playerID,
	})
}

// respondOnboardingError は onboarding usecase の sentinel を HTTP status にマップする。
func respondOnboardingError(c *gin.Context, err error) {
	c.JSON(onboardingErrorStatus(err), gin.H{"error": err.Error()})
}

// onboardingErrorStatus は onboarding 特有のエラーを HTTP status に翻訳する。
func onboardingErrorStatus(err error) int {
	switch {
	case errors.Is(err, onboarding.ErrScriptNotFound),
		errors.Is(err, onboarding.ErrPlayerNotFound):
		return http.StatusNotFound
	case errors.Is(err, onboarding.ErrAlreadyOnboarded),
		errors.Is(err, onboarding.ErrFactionNotSelected):
		return http.StatusConflict
	case errors.Is(err, onboarding.ErrInvalidFaction),
		errors.Is(err, onboarding.ErrInvalidName):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// isOnboardingRejected は仕様通りの拒否か判定する (handler のログレベルを info に落とすために使う)。
func isOnboardingRejected(err error) bool {
	return errors.Is(err, onboarding.ErrAlreadyOnboarded) ||
		errors.Is(err, onboarding.ErrScriptNotFound) ||
		errors.Is(err, onboarding.ErrInvalidFaction) ||
		errors.Is(err, onboarding.ErrInvalidName) ||
		errors.Is(err, onboarding.ErrPlayerNotFound) ||
		errors.Is(err, onboarding.ErrFactionNotSelected)
}

// logGetOnboardingScriptError は GetScript のエラー種別ごとのログレベルを決定する。
func logGetOnboardingScriptError(err error, playerID, lang string) {
	attrs := []any{"error", err, "player_id", playerID, "lang", lang}
	switch {
	case isOnboardingRejected(err):
		slog.Info("get onboarding script rejected", attrs...)
	default:
		slog.Error("get onboarding script failed", attrs...)
	}
}
