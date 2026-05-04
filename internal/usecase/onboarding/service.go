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
	"github.com/kenyamaneko/overload-party-scenario/internal/presenter"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// Service はオンボーディングのユースケースを束ねる。
type Service struct {
	repo          port.OnboardingRepo
	scriptStore   port.ScriptStore
	nameValidator port.OnboardingNameValidator
	playerReader  port.OnboardingPlayerReader
}

// New は Service を構築する。
func New(
	repo port.OnboardingRepo,
	scriptStore port.ScriptStore,
	nameValidator port.OnboardingNameValidator,
	playerReader port.OnboardingPlayerReader,
) *Service {
	return &Service{
		repo:          repo,
		scriptStore:   scriptStore,
		nameValidator: nameValidator,
		playerReader:  playerReader,
	}
}

// GetScript はオンボーディングシナリオ本文を返す。
func (s *Service) GetScript(ctx context.Context, playerID, lang string) (string, error) {
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

// UpdateName はオンボード内 name 入力ステップを処理する。
func (s *Service) UpdateName(ctx context.Context, playerID, name string) error {
	if err := s.nameValidator.ValidateOnboardingName(ctx, playerID, name); err != nil {
		if errors.Is(err, port.ErrInvalidName) {
			return ErrInvalidName
		}
		if errors.Is(err, port.ErrPlayerNotFound) {
			return ErrPlayerNotFound
		}
		return fmt.Errorf("validate onboarding name: %w", err)
	}

	ev, err := buildOnboardingNameSetEvent(playerID, name)
	if err != nil {
		return fmt.Errorf("build onboarding-name-set: %w", err)
	}
	if err := s.repo.PublishEvents(ctx, ev); err != nil {
		return fmt.Errorf("publish onboarding-name-set: %w", err)
	}
	return nil
}

// SelectFaction はオンボード内 faction 選択ステップを処理する。
func (s *Service) SelectFaction(ctx context.Context, playerID, initialFactionID string) error {
	if err := validateFaction(initialFactionID); err != nil {
		return err
	}

	ev, err := buildOnboardingFactionSetEvent(playerID, initialFactionID)
	if err != nil {
		return fmt.Errorf("build onboarding-faction-set: %w", err)
	}
	if err := s.repo.PublishEvents(ctx, ev); err != nil {
		return fmt.Errorf("publish onboarding-faction-set: %w", err)
	}
	return nil
}

// Complete はオンボーディング完了を記録し、player-onboarded event を outbox に同一 tx で積む。
func (s *Service) Complete(ctx context.Context, playerID string) error {
	player, err := s.playerReader.GetOnboardingPlayer(ctx, playerID)
	if err != nil {
		if errors.Is(err, port.ErrPlayerNotFound) {
			return ErrPlayerNotFound
		}
		return fmt.Errorf("get onboarding player: %w", err)
	}
	if player.SelectedFaction == nil || *player.SelectedFaction == "" {
		return ErrFactionNotSelected
	}

	ev, err := buildPlayerOnboardedEvent(playerID, *player.SelectedFaction)
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
func validateFaction(factionID string) error {
	if !slices.Contains(gamedesign.SelectableFactions, factionID) {
		return ErrInvalidFaction
	}
	return nil
}

// buildOnboardingNameSetEvent は onboarding-name-set の outbox 行を構築する。
// presenter で wire 表現を組み立て、ここで marshal + outbox 行への wrap を担う。
func buildOnboardingNameSetEvent(playerID, name string) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("onboarding: playerID is empty")
	}
	if name == "" {
		return port.OutboxEvent{}, errors.New("onboarding: name is empty")
	}
	eventID := uuid.New()
	ev := presenter.ToOnboardingNameSetEvent(eventID.String(), playerID, name, time.Now().UTC())
	payload, err := json.Marshal(ev)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("marshal onboarding-name-set: %w", err)
	}
	return port.OutboxEvent{
		EventID:   eventID,
		EventType: apiscenario.EventTypeOnboardingNameSet,
		Payload:   payload,
	}, nil
}

// buildOnboardingFactionSetEvent は onboarding-faction-set の outbox 行を構築する。
func buildOnboardingFactionSetEvent(playerID, initialFactionID string) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("onboarding: playerID is empty")
	}
	if initialFactionID == "" {
		return port.OutboxEvent{}, errors.New("onboarding: initialFactionID is empty")
	}
	eventID := uuid.New()
	ev := presenter.ToOnboardingFactionSetEvent(eventID.String(), playerID, initialFactionID, time.Now().UTC())
	payload, err := json.Marshal(ev)
	if err != nil {
		return port.OutboxEvent{}, fmt.Errorf("marshal onboarding-faction-set: %w", err)
	}
	return port.OutboxEvent{
		EventID:   eventID,
		EventType: apiscenario.EventTypeOnboardingFactionSet,
		Payload:   payload,
	}, nil
}

// buildPlayerOnboardedEvent は player-onboarded の outbox 行を構築する。
func buildPlayerOnboardedEvent(playerID, initialFactionID string) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("onboarding: playerID is empty")
	}
	if initialFactionID == "" {
		return port.OutboxEvent{}, errors.New("onboarding: initialFactionID is empty")
	}
	eventID := uuid.New()
	ev := presenter.ToPlayerOnboardedEvent(eventID.String(), playerID, initialFactionID, time.Now().UTC())
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
