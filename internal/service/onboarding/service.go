package onboarding

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	gamedesign "github.com/kenyamaneko/overload-party-common/packages/game-design-constants"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

// displayNameMaxRunes は display_name の最大長 (rune 単位)。
// 日本語・絵文字・合成文字を考慮し byte ではなく rune でカウントする。
// MVP の仕様として 21 rune 以下とする。
const displayNameMaxRunes = 21

// Service はオンボーディングのユースケースを束ねる。
//
// scriptStore は起動時 (config 判定) に一度だけ決定される (GCS か local filesystem)。
// repo と eventBuilder は外部永続装置への口であり、ロジックは持たない。
type Service struct {
	repo         port.OnboardingRepo
	scriptStore  port.ScriptStore
	eventBuilder port.OutboxEventBuilder
}

// New は Service を構築する。
func New(repo port.OnboardingRepo, scriptStore port.ScriptStore, eventBuilder port.OutboxEventBuilder) *Service {
	return &Service{
		repo:         repo,
		scriptStore:  scriptStore,
		eventBuilder: eventBuilder,
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
		// Why: script_store adapter はネイティブの "not found" を
		// port.ErrScriptNotFound に変換する契約。ここでは更に onboarding 固有の
		// sentinel (ErrScriptNotFound) に翻訳し、handler の分類ロジックを明確化する。
		if errors.Is(err, port.ErrScriptNotFound) {
			return "", ErrScriptNotFound
		}
		return "", fmt.Errorf("read onboarding script: %w", err)
	}
	return body, nil
}

// Complete はオンボーディング完了を記録し、player-onboarded と faction-selected を
// outbox に atomic に積む。下流は outbox worker が Pub/Sub に publish する。
//
// display_name / initial_faction_id のバリデーションは service 層で行い、
// outbox に不正データが乗らないようにする。二度目以降の完了は repo 層が
// ErrAlreadyOnboarded に classify し、ここで onboarding 固有の sentinel に翻訳する。
func (s *Service) Complete(ctx context.Context, playerID, displayName, initialFactionID string) error {
	if err := validateDisplayName(displayName); err != nil {
		return err
	}
	if err := validateFaction(initialFactionID); err != nil {
		return err
	}

	evOnboarded, err := s.eventBuilder.BuildPlayerOnboarded(playerID, displayName, initialFactionID)
	if err != nil {
		return fmt.Errorf("build player-onboarded: %w", err)
	}
	evFaction, err := s.eventBuilder.BuildFactionSelected(playerID, initialFactionID)
	if err != nil {
		return fmt.Errorf("build faction-selected: %w", err)
	}

	if err := s.repo.MarkComplete(ctx, playerID, evOnboarded, evFaction); err != nil {
		if errors.Is(err, port.ErrAlreadyOnboarded) {
			return ErrAlreadyOnboarded
		}
		return fmt.Errorf("mark complete: %w", err)
	}
	return nil
}

// validateDisplayName は MVP 仕様に沿って display_name を検証する。
// 空文字 / 全文字が whitespace のみ / 21 rune 超過 はいずれも ErrInvalidDisplayName。
func validateDisplayName(name string) error {
	if name == "" {
		return ErrInvalidDisplayName
	}
	if strings.TrimSpace(name) == "" {
		return ErrInvalidDisplayName
	}
	if utf8.RuneCountInString(name) > displayNameMaxRunes {
		return ErrInvalidDisplayName
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
