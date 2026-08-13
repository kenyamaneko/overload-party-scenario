package presenter

import (
	"fmt"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// ToLockReason は domain.LockReason を wire の apiscenario.LockReason に詰め替える。
// type ごとに有効なフィールドのみ埋める (他 variant のフィールドは nil で透過する)。
func ToLockReason(r domain.LockReason) apiscenario.LockReason {
	switch r.Type {
	case domain.LockReasonLevel:
		required := r.RequiredLevel
		current := r.CurrentLevel
		return apiscenario.LockReason{
			Type:          apiscenario.LockReasonTypeLevel,
			RequiredLevel: &required,
			CurrentLevel:  &current,
		}
	case domain.LockReasonFaction:
		faction := r.RequiredFaction
		return apiscenario.LockReason{
			Type:            apiscenario.LockReasonTypeFaction,
			RequiredFaction: &faction,
		}
	case domain.LockReasonEpisode:
		episode := r.RequiredEpisode
		return apiscenario.LockReason{
			Type:            apiscenario.LockReasonTypeEpisode,
			RequiredEpisode: &episode,
		}
	default:
		panic(fmt.Sprintf("presenter: 未知の LockReasonType %q", r.Type))
	}
}

// ToLockReasons は domain.LockReason slice を wire LockReason slice に詰め替える。
// 0 件のときも空 slice を返す (EpisodeWithStatus.lock_reasons は required 配列であり、
// JSON シリアライズ時に null でなく [] になる必要があるため)。
func ToLockReasons(reasons []domain.LockReason) []apiscenario.LockReason {
	result := make([]apiscenario.LockReason, len(reasons))
	for i, r := range reasons {
		result[i] = ToLockReason(r)
	}
	return result
}

// BuildEpisodeWithStatus は Episode と UnlockContext からアンロック状態と
// ロック理由を派生させて wire の EpisodeWithStatus を組み立てる。
func BuildEpisodeWithStatus(ep *domain.Episode, uc *domain.UnlockContext, lang string) apiscenario.EpisodeWithStatus {
	reasons := ep.LockReasons(uc)
	return apiscenario.EpisodeWithStatus{
		EpisodeID:     ep.EpisodeID,
		Faction:       ep.Faction,
		EpisodeNumber: ep.EpisodeNumber,
		Title:         ep.Title(lang),
		ThumbnailURL:  ep.ThumbnailPath,
		IsCompleted:   uc.CompletedEpisodes[ep.EpisodeID],
		IsUnlocked:    len(reasons) == 0,
		LockReasons:   ToLockReasons(reasons),
	}
}

// BuildEpisodesWithStatus は Episode slice を共通の UnlockContext に対して評価し、
// EpisodeWithStatus slice として返す。
func BuildEpisodesWithStatus(episodes []*domain.Episode, uc *domain.UnlockContext, lang string) []apiscenario.EpisodeWithStatus {
	result := make([]apiscenario.EpisodeWithStatus, len(episodes))
	for i, ep := range episodes {
		result[i] = BuildEpisodeWithStatus(ep, uc, lang)
	}
	return result
}
