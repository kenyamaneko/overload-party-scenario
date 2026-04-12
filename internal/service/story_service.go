package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

var (
	// ErrEpisodeNotFound はエピソードが存在しないか非アクティブの場合に返される。
	ErrEpisodeNotFound = errors.New("episode not found")
	// ErrEpisodeLocked はアンロック条件を満たさないエピソードへのアクセスで返される。
	ErrEpisodeLocked = errors.New("episode is locked")
)

const fallbackLang = "ja"

// StoryService はリポジトリからエピソードメタデータを、ScriptStore アダプタ（GCS、
// ローカルファイルシステム等）からストーリースクリプトを読み込む。スクリプトソースは
// 構築時（config に基づいて）に一度だけ決定される。
type StoryService struct {
	storyRepo   port.StoryRepo
	scriptStore port.ScriptStore

	// 初期 faction 選択イベントを Pub/Sub 経由で fan-out する。
	// account / card / gateway がそれぞれ subscribe し独立に更新を commit する。
	// hand-off パスをテストしない場合は nil。NotifyInitialFactionSelected は明示的に
	// エラーを返すため配線の退行を即座に検知できる。
	factionPublisher port.FactionPublisher
}

// NewStoryService は StoryService を構築する。scriptStore はエピソードメタデータのみを
// テストする場合 nil 可（GetScript を呼ぶと panic する）。factionPublisher は
// faction hand-off パスをテストしない場合 nil 可。
func NewStoryService(storyRepo port.StoryRepo, scriptStore port.ScriptStore, factionPublisher port.FactionPublisher) *StoryService {
	return &StoryService{
		storyRepo:        storyRepo,
		scriptStore:      scriptStore,
		factionPublisher: factionPublisher,
	}
}

// ListEpisodes はプレイヤー向けエピソード一覧をアンロック状態付きで返す。
func (s *StoryService) ListEpisodes(ctx context.Context, playerID, lang string) ([]apiscenario.EpisodeWithStatus, error) {
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
		status := apiscenario.EpisodeWithStatus{
			EpisodeID:     ep.EpisodeID,
			Faction:       ep.Faction,
			EpisodeNumber: ep.EpisodeNumber,
			Title:         episodeTitle(ep, lang),
			IsCompleted:   uc.CompletedEpisodes[ep.EpisodeID],
			IsUnlocked:    len(reasons) == 0,
			LockReasons:   reasons,
		}
		result = append(result, status)
	}
	return result, nil
}

// GetScript は指定エピソードのスクリプト本文を返す。
func (s *StoryService) GetScript(ctx context.Context, playerID, episodeID, lang string) (string, error) {
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

	script, err := s.readScript(ctx, ep.ScriptPath, lang)
	if err != nil {
		return "", err
	}
	return script, nil
}

// CompleteEpisode はエピソードの完了を記録する。
func (s *StoryService) CompleteEpisode(ctx context.Context, playerID, episodeID string) error {
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

	// TODO(faction-handoff): プレイヤーの初期 faction を確定するチュートリアルエピソード
	// 完了時に s.NotifyInitialFactionSelected(ctx, playerID, <faction>) を呼ぶ。
	// scenario_episodes にチュートリアルを識別するデータ（`grants_initial_faction`
	// カラム等）が必要。データ整備後にユーザーが手動で配線するか後続 ADR で対応。
	return nil
}

// NotifyInitialFactionSelected は初期 faction 選択フロー用に `faction-selected`
// Pub/Sub イベントを発行する。account（player_factions + players.selected_faction 書き込み）、
// card（player_cards 書き込み）、gateway（WS 完了通知 push）がそれぞれ subscribe する。
// scenario は subscriber を待たず、REST handler は Pub/Sub ack 後に 202 Accepted を返し、
// クライアントは gateway WS push を待つ。
//
// べき等性: subscriber は `<schema>.processed_events`（event_id キー）で重複を排除する。
// チュートリアル再プレイ時は新しい event_id が生成されるが、subscriber は player_factions
// の複合 PK により二重挿入を no-op する。
func (s *StoryService) NotifyInitialFactionSelected(ctx context.Context, playerID, factionID string) error {
	if s.factionPublisher == nil {
		return fmt.Errorf("scenario: NotifyInitialFactionSelected called with nil factionPublisher")
	}
	if err := s.factionPublisher.PublishFactionSelected(ctx, playerID, factionID); err != nil {
		return fmt.Errorf("publish faction-selected: %w", err)
	}
	return nil
}

func (s *StoryService) validateUnlock(ctx context.Context, ep *apiscenario.ScenarioEpisode, playerID string) error {
	uc, err := s.storyRepo.GetUnlockContext(ctx, playerID)
	if err != nil {
		return fmt.Errorf("get unlock context: %w", err)
	}

	if reasons := checkUnlock(ep, uc); len(reasons) > 0 {
		return ErrEpisodeLocked
	}
	return nil
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

// readScript は設定済み ScriptStore から (pathTemplate, lang) のスクリプト本文を読み込む。
// 3 つの結果を区別する:
//
//   - 成功: 要求言語が存在するか、存在しないが `ja` フォールバックが存在する。
//   - not-found: 要求言語も `ja` も存在しない。port.ErrScriptNotFound を返す。
//   - infra: それ以外のストレージエラー。port.ErrScriptInfra を返す。
func (s *StoryService) readScript(ctx context.Context, pathTemplate, lang string) (string, error) {
	key := strings.Replace(pathTemplate, "{lang}", lang, 1)
	body, err := s.scriptStore.ReadScript(ctx, key)
	if err == nil {
		return body, nil
	}

	// not-found のみフォールバック対象。infra エラーは即座に伝播する。
	if !errors.Is(err, port.ErrScriptNotFound) {
		return "", err
	}

	if lang == fallbackLang {
		return "", err
	}

	jaKey := strings.Replace(pathTemplate, "{lang}", fallbackLang, 1)
	return s.scriptStore.ReadScript(ctx, jaKey)
}

func episodeTitle(ep *apiscenario.ScenarioEpisode, lang string) string {
	if lang == "en" {
		return ep.TitleEn
	}
	return ep.TitleJa
}
