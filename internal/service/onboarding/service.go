package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	gamedesign "github.com/kenyamaneko/overload-party-common/packages/game-design-constants"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// displayNameMaxRunes は display_name の最大長 (rune 単位)。
// 日本語・絵文字・合成文字を考慮し byte ではなく rune でカウントする。
// MVP の仕様として 21 rune 以下とする。
const displayNameMaxRunes = 21

// Service はオンボーディングのユースケースを束ねる。
//
// scriptStore は起動時 (config 判定) に一度だけ決定される (GCS か local filesystem)。
// repo は外部永続装置への口であり、ロジックは持たない。outbox イベントの
// 構築は service 層の純粋関数 (buildPlayerOnboardedEvent) で行う。
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

// Complete はオンボーディング完了を記録し、player-onboarded を outbox に atomic に
// 積む。下流は outbox worker が Pub/Sub に publish し、各 subscriber
// (account / card / gateway …) が display_name と initial_faction を自スキーマへ
// 反映する (ADR-022: faction-selected は player-onboarded に統合)。
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

	ev, err := buildPlayerOnboardedEvent(playerID, displayName, initialFactionID)
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

// buildPlayerOnboardedEvent は player-onboarded の outbox 行 payload を構築する。
// イベント struct スキーマ (apiscenario.PlayerOnboardedEvent) を service 層に閉じ込め、
// adapter は payload を不透明な []byte として送出する。EventID は payload 内
// eventId と outbox 行の PK の双方に使い、subscriber が冪等性キーとして使える。
func buildPlayerOnboardedEvent(playerID, displayName, initialFactionID string) (port.OutboxEvent, error) {
	if playerID == "" {
		return port.OutboxEvent{}, errors.New("onboarding: playerID is empty")
	}
	if displayName == "" {
		return port.OutboxEvent{}, errors.New("onboarding: displayName is empty")
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
		DisplayName:      displayName,
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
