package rest

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
	"github.com/kenyamaneko/overload-party-scenario/internal/service/onboarding"
)

// OnboardingHandler はオンボーディング (初回プロローグ) の REST ハンドラを提供する。
type OnboardingHandler struct {
	svc *onboarding.Service
}

// NewOnboardingHandler は OnboardingHandler を構築する。
func NewOnboardingHandler(svc *onboarding.Service) *OnboardingHandler {
	return &OnboardingHandler{svc: svc}
}

// GetStatus はプレイヤーのオンボーディング完了状態を返す。
// GET /internal/v1/players/:playerId/onboarding/status
func (h *OnboardingHandler) GetStatus(c *gin.Context) {
	playerID := c.Param("playerId")
	if playerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "playerId is required"})
		return
	}

	status, err := h.svc.GetStatus(c.Request.Context(), playerID)
	if err != nil {
		slog.Error("get onboarding status failed", "error", err, "player_id", playerID)
		respondOnboardingError(c, err)
		return
	}

	c.JSON(http.StatusOK, apiscenario.OnboardingStatus{
		PlayerID:    playerID,
		Onboarded:   status.Onboarded,
		CompletedAt: status.CompletedAt,
	})
}

// GetScript はオンボーディングシナリオ本文を返す。
// GET /internal/v1/players/:playerId/onboarding/script?lang=ja|en
func (h *OnboardingHandler) GetScript(c *gin.Context) {
	playerID := c.Param("playerId")
	if playerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "playerId is required"})
		return
	}
	lang := c.DefaultQuery("lang", "ja")

	body, err := h.svc.GetScript(c.Request.Context(), playerID, lang)
	if err != nil {
		logGetOnboardingScriptError(err, playerID, lang)
		respondOnboardingError(c, err)
		return
	}

	c.JSON(http.StatusOK, apiscenario.OnboardingScriptResponse{Script: body})
}

// Complete はオンボーディング完了を記録し、outbox に player-onboarded と
// faction-selected を atomic に積む。
// POST /internal/v1/players/:playerId/onboarding/complete
func (h *OnboardingHandler) Complete(c *gin.Context) {
	playerID := c.Param("playerId")
	if playerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "playerId is required"})
		return
	}

	var req apiscenario.OnboardingCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Info("complete onboarding bind failed", "error", err, "player_id", playerID)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.Complete(c.Request.Context(), playerID, req.DisplayName, req.InitialFactionID); err != nil {
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

// respondOnboardingError は onboarding service の sentinel を HTTP status にマップする。
// 既存 respondError の分類 (story 用) を流用すると "completed → 404" のように
// 誤分類するため、onboarding handler 専用の分類関数を分けて責務を閉じる。
func respondOnboardingError(c *gin.Context, err error) {
	c.JSON(onboardingErrorStatus(err), gin.H{"error": err.Error()})
}

// onboardingErrorStatus は onboarding 特有のエラーを HTTP status に翻訳する。
// default は 500 (DB 一時障害・未分類) でクライアント側のリトライに委ねる。
func onboardingErrorStatus(err error) int {
	switch {
	case errors.Is(err, onboarding.ErrScriptNotFound):
		return http.StatusNotFound
	case errors.Is(err, onboarding.ErrAlreadyOnboarded):
		return http.StatusConflict
	case errors.Is(err, onboarding.ErrInvalidDisplayName),
		errors.Is(err, onboarding.ErrInvalidFaction):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// isOnboardingRejected は仕様通りの拒否 (クライアントの入力不備 / 既に完了済み /
// スクリプト不在) か判定する。これらはログレベルを info に落とす。
func isOnboardingRejected(err error) bool {
	return errors.Is(err, onboarding.ErrAlreadyOnboarded) ||
		errors.Is(err, onboarding.ErrScriptNotFound) ||
		errors.Is(err, onboarding.ErrInvalidDisplayName) ||
		errors.Is(err, onboarding.ErrInvalidFaction)
}

// logGetOnboardingScriptError は GetScript のエラー種別ごとのログレベルを決定する。
// 仕様通りの拒否は info、それ以外は error で記録する。
func logGetOnboardingScriptError(err error, playerID, lang string) {
	attrs := []any{"error", err, "player_id", playerID, "lang", lang}
	switch {
	case isOnboardingRejected(err):
		slog.Info("get onboarding script rejected", attrs...)
	default:
		slog.Error("get onboarding script failed", attrs...)
	}
}
