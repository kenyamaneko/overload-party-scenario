package story

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// Service はエピソード一覧取得・スクリプト取得・完了記録を束ねるストーリーのユースケース。
type Service struct {
	storyRepo   port.StoryRepo
	scriptStore port.ScriptStore
}

// New は Service を構築する。
func New(storyRepo port.StoryRepo, scriptStore port.ScriptStore) *Service {
	return &Service{
		storyRepo:   storyRepo,
		scriptStore: scriptStore,
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

func (s *Service) validateUnlock(ctx context.Context, ep *domain.Episode, playerID string) error {
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
func (s *Service) readScript(ctx context.Context, pathTemplate, lang string) (string, error) {
	key := strings.ReplaceAll(pathTemplate, "{lang}", lang)
	body, err := s.scriptStore.ReadScript(ctx, key)
	if err != nil {
		return "", fmt.Errorf("read script: %w", err)
	}
	return body, nil
}

// checkUnlock は domain の Episode と UnlockContext から API 応答用の LockReason 群を組み立てる。
// LockReason は API 契約 (apiscenario) に直結する出力なので usecase でそのまま生成して返す。
func checkUnlock(ep *domain.Episode, uc *domain.UnlockContext) []apiscenario.LockReason {
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

func episodeTitle(ep *domain.Episode, lang string) string {
	if lang == "en" {
		return ep.TitleEn
	}
	return ep.TitleJa
}
