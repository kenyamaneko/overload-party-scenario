package story

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// Service はエピソード一覧取得・スクリプト取得・完了記録・初期 faction 通知を束ねる
// ストーリーのユースケース実装。
//
// scriptStore は起動時 (config 判定) に一度だけ決定される (GCS か local filesystem)。
// factionPublisher は faction hand-off パスを動かさないテストでは nil 可で、
// NotifyInitialFactionSelected が nil 時に明示的エラーを返すことで配線退行を即検知する。
type Service struct {
	storyRepo        port.StoryRepo
	scriptStore      port.ScriptStore
	factionPublisher port.FactionPublisher
}

// New は Service を構築する。
func New(storyRepo port.StoryRepo, scriptStore port.ScriptStore, factionPublisher port.FactionPublisher) *Service {
	return &Service{
		storyRepo:        storyRepo,
		scriptStore:      scriptStore,
		factionPublisher: factionPublisher,
	}
}

// ListEpisodes はプレイヤー向けエピソード一覧をアンロック状態付きで返す。
func (s *Service) ListEpisodes(ctx context.Context, playerID, lang string) ([]apiscenario.EpisodeWithStatus, error) {
	episodes, err := s.storyRepo.ListActiveEpisodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list episodes: %w", err)
	}

	uc, err := s.storyRepo.GetUnlockContext(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get unlock context: %w", err)
	}

	result := make([]apiscenario.EpisodeWithStatus, 0, len(episodes))
	for _, ep := range episodes {
		reasons := checkUnlock(ep, uc)
		result = append(result, apiscenario.EpisodeWithStatus{
			EpisodeID:     ep.EpisodeID,
			Faction:       ep.Faction,
			EpisodeNumber: ep.EpisodeNumber,
			Title:         episodeTitle(ep, lang),
			IsCompleted:   uc.CompletedEpisodes[ep.EpisodeID],
			IsUnlocked:    len(reasons) == 0,
			LockReasons:   reasons,
		})
	}
	return result, nil
}

// GetScript は指定エピソードのスクリプト本文を返す。
func (s *Service) GetScript(ctx context.Context, playerID, episodeID, lang string) (string, error) {
	ep, err := s.storyRepo.FindEpisodeByID(ctx, episodeID)
	if err != nil {
		if errors.Is(err, port.ErrNotFound) {
			return "", ErrEpisodeNotFound
		}
		return "", fmt.Errorf("find episode: %w", err)
	}
	if !ep.IsActive {
		return "", ErrEpisodeNotFound
	}

	if err := s.validateUnlock(ctx, ep, playerID); err != nil {
		return "", err
	}

	return s.readScript(ctx, ep.ScriptPath, lang)
}

// CompleteEpisode はエピソードの完了を記録する。
func (s *Service) CompleteEpisode(ctx context.Context, playerID, episodeID string) error {
	ep, err := s.storyRepo.FindEpisodeByID(ctx, episodeID)
	if err != nil {
		if errors.Is(err, port.ErrNotFound) {
			return ErrEpisodeNotFound
		}
		return fmt.Errorf("find episode: %w", err)
	}
	if !ep.IsActive {
		return ErrEpisodeNotFound
	}

	if err := s.validateUnlock(ctx, ep, playerID); err != nil {
		return err
	}

	if err := s.storyRepo.MarkComplete(ctx, playerID, episodeID); err != nil {
		return fmt.Errorf("mark complete: %w", err)
	}
	return nil
}

// NotifyInitialFactionSelected は初期 faction 選択フロー用に faction-selected Pub/Sub
// イベントを発行する。account / card / gateway がそれぞれ subscribe し、
// subscriber 側は <schema>.processed_events (event_id キー) で重複排除する。
//
// factionPublisher が nil の場合は明示的エラーを返し、配線の退行を即検知させる。
func (s *Service) NotifyInitialFactionSelected(ctx context.Context, playerID, factionID string) error {
	if s.factionPublisher == nil {
		return fmt.Errorf("scenario: NotifyInitialFactionSelected called with nil factionPublisher")
	}
	if err := s.factionPublisher.PublishFactionSelected(ctx, playerID, factionID); err != nil {
		return fmt.Errorf("publish faction-selected: %w", err)
	}
	return nil
}

func (s *Service) validateUnlock(ctx context.Context, ep *apiscenario.ScenarioEpisode, playerID string) error {
	uc, err := s.storyRepo.GetUnlockContext(ctx, playerID)
	if err != nil {
		return fmt.Errorf("get unlock context: %w", err)
	}
	if reasons := checkUnlock(ep, uc); len(reasons) > 0 {
		return ErrEpisodeLocked
	}
	return nil
}

// readScript は pathTemplate の {lang} を指定言語で置換し ScriptStore から読む。
// 要求言語のスクリプトが存在しなければ ErrScriptNotFound を返す（代替言語へフォールバックしない）。
func (s *Service) readScript(ctx context.Context, pathTemplate, lang string) (string, error) {
	key := strings.ReplaceAll(pathTemplate, "{lang}", lang)
	body, err := s.scriptStore.ReadScript(ctx, key)
	if err != nil {
		return "", fmt.Errorf("read script: %w", err)
	}
	return body, nil
}

func checkUnlock(ep *apiscenario.ScenarioEpisode, uc *apiscenario.StoryUnlockContext) []apiscenario.LockReason {
	var reasons []apiscenario.LockReason

	if uc.PlayerLevel < ep.RequiredLevel {
		reasons = append(reasons, apiscenario.NewLockReasonLevel(ep.RequiredLevel, uc.PlayerLevel))
	}
	for _, f := range ep.RequiredFactions {
		if !uc.OwnedFactions[f] {
			reasons = append(reasons, apiscenario.NewLockReasonFaction(f))
		}
	}
	for _, reqEp := range ep.RequiredEpisodes {
		if !uc.CompletedEpisodes[reqEp] {
			reasons = append(reasons, apiscenario.NewLockReasonEpisode(reqEp))
		}
	}
	return reasons
}

func episodeTitle(ep *apiscenario.ScenarioEpisode, lang string) string {
	if lang == "en" {
		return ep.TitleEn
	}
	return ep.TitleJa
}
