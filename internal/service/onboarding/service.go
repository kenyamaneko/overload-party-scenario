package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	gamedesign "github.com/kenyamaneko/overload-party-common/packages/game-design-constants"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// Service はオンボーディングのユースケースを束ねる。
type Service struct {
	repo        port.OnboardingRepo
	scriptStore port.ScriptStore
}

// New は Service を構築する。
func New(repo port.OnboardingRepo, scriptStore port.ScriptStore) *Service {
	return &Service{
		repo:        repo,
		scriptStore: scriptStore,
	}
}

// GetStatus はプレイヤーのオンボーディング進捗を返す。
func (s *Service) GetStatus(ctx context.Context, playerID string) (port.OnboardingStatus, error) {
	status, err := s.repo.GetStatus(ctx, playerID)
	if err != nil {
		return port.OnboardingStatus{}, fmt.Errorf("get onboarding status: %w", err)
	}
	return status, nil
}

// GetScript はオンボーディングシナリオ本文を返す。
// 完了済みなら ErrAlreadyOnboarded を返し (再読み込みは UI 側で status 判定して防ぐ)、
// 要求言語のスクリプトが存在しなければ ErrScriptNotFound を返す (代替言語へフォールバックしない)。
func (s *Service) GetScript(ctx context.Context, playerID, lang string) (string, error) {
	status, err := s.repo.GetStatus(ctx, playerID)
	if err != nil {
		return "", fmt.Errorf("get onboarding status: %w", err)
	}
	if status.Onboarded {
		return "", ErrAlreadyOnboarded
	}

	key := fmt.Sprintf("scripts/onboarding/%s.ks", lang)
	body, err := s.scriptStore.ReadScript(ctx, key)
	if err != nil {
		if errors.Is(err, port.ErrScriptNotFound) {
			return "", ErrScriptNotFound
		}
		return "", fmt.Errorf("read onboarding script: %w", err)
	}
	return body, nil
}

// Complete はオンボーディング完了を記録し、player-onboarded を outbox に同一 tx で積む。
// 二度目以降の完了は ErrAlreadyOnboarded を返す。
// 表示名はオンボード内 name 入力ステップで account に確定済みのため、本フローでは扱わない。
func (s *Service) Complete(ctx context.Context, playerID, initialFactionID string) error {
	if err := validateFaction(initialFactionID); err != nil {
		return err
	}

	ev, err := buildPlayerOnboardedEvent(playerID, initialFactionID)
	if err != nil {
		return fmt.Errorf("build player-onboarded: %w", err)
	}

	if err := s.repo.MarkComplete(ctx, playerID, ev); err != nil {
		if errors.Is(err, port.ErrAlreadyOnboarded) {
			return ErrAlreadyOnboarded
		}
		return fmt.Errorf("mark complete: %w", err)
	}
	return nil
}

// validateFaction は initial_faction_id が SelectableFactions に含まれるか検証する。
// gamedesign.SelectableFactions が game-design-constants から注入される値の SSoT。
func validateFaction(factionID string) error {
	if !slices.Contains(gamedesign.SelectableFactions, factionID) {
		return ErrInvalidFaction
	}
	return nil
}

// buildPlayerOnboardedEvent は player-onboarded の outbox 行 payload を構築する。
// EventID は payload 内 eventId と outbox 行 PK の双方に使い、subscriber が
// 冪等性キーとして使える。
func buildPlayerOnboardedEvent(playerID, initialFactionID string) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("onboarding: playerID is empty")
	}
	if initialFactionID == "" {
		return port.OutboxEvent{}, errors.New("onboarding: initialFactionID is empty")
	}
	eventID := uuid.New()
	ev := apiscenario.PlayerOnboardedEvent{
		EventType:        apiscenario.EventTypePlayerOnboarded,
		EventID:          eventID.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		InitialFactionID: initialFactionID,
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("marshal player-onboarded: %w", err)
	}
	return port.OutboxEvent{
		EventID:   eventID,
		EventType: apiscenario.EventTypePlayerOnboarded,
		Payload:   payload,
	}, nil
}
