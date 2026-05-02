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
// account への REST 呼び出しは validate と GetPlayer の 2 経路に限定し、
// 業務データの永続化はすべて scenario の outbox publish 経由で account に依頼する。
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
// 要求言語のスクリプトが存在しなければ ErrScriptNotFound を返す
// (代替言語へフォールバックしない)。
//
// 完了済み判定 (ErrAlreadyOnboarded) は scenario 側の責務として scenario.player_onboarding
// 完了行の存在を確認する形で行う。完了状態の取得経路は account の GetPlayer
// (onboarding_status) 経由が SSoT だが、ここでは GCS フェッチ前の short-circuit に必要な
// 局所判定として完了マーク行の存在のみを利用する。
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
// account に validate を REST で依頼し、成功時に onboarding-name-set event を
// outbox に積む。name の永続化は account 側 subscriber が同一 tx で実行する。
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
// SelectableFactions に対する検証を scenario 側で行い、成功時に
// onboarding-faction-set event を outbox に積む。faction の永続化は account 側
// subscriber が同一 tx で実行する。
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

// Complete はオンボーディング完了を記録し、player-onboarded event を outbox に
// 同一 tx で積む。二度目以降の完了は ErrAlreadyOnboarded を返す。
//
// PlayerOnboardedEvent payload の InitialFactionID は account から取得する
// (faction 選択ステップで account.players.selected_faction に書き込み済み)。
// account 側に未設定なら ErrFactionNotSelected を返し、faction 選択を経ずに
// 完了 API が叩かれたフロー違反を 409 として伝播する。
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
// gamedesign.SelectableFactions が game-design-constants から注入される値の SSoT。
func validateFaction(factionID string) error {
	if !slices.Contains(gamedesign.SelectableFactions, factionID) {
		return ErrInvalidFaction
	}
	return nil
}

// buildOnboardingNameSetEvent は onboarding-name-set の outbox 行 payload を構築する。
// EventID は payload 内 eventId と outbox 行 PK の双方に使い、subscriber が
// 冪等性キーとして使える。
func buildOnboardingNameSetEvent(playerID, name string) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("onboarding: playerID is empty")
	}
	if name == "" {
		return port.OutboxEvent{}, errors.New("onboarding: name is empty")
	}
	eventID := uuid.New()
	ev := apiscenario.OnboardingNameSetEvent{
		EventType: apiscenario.EventTypeOnboardingNameSet,
		EventID:   eventID.String(),
		Timestamp: time.Now().UTC(),
		PlayerID:  playerID,
		Name:      name,
	}
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

// buildOnboardingFactionSetEvent は onboarding-faction-set の outbox 行 payload を構築する。
func buildOnboardingFactionSetEvent(playerID, initialFactionID string) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("onboarding: playerID is empty")
	}
	if initialFactionID == "" {
		return port.OutboxEvent{}, errors.New("onboarding: initialFactionID is empty")
	}
	eventID := uuid.New()
	ev := apiscenario.OnboardingFactionSetEvent{
		EventType:        apiscenario.EventTypeOnboardingFactionSet,
		EventID:          eventID.String(),
		Timestamp:        time.Now().UTC(),
		PlayerID:         playerID,
		InitialFactionID: initialFactionID,
	}
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

// buildPlayerOnboardedEvent は player-onboarded の outbox 行 payload を構築する。
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
