package presenter

import (
	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// ToLockReason は domain.LockReason を wire の apiscenario.LockReason に詰め替えます。
func ToLockReason(r domain.LockReason) apiscenario.LockReason {
	switch r.Type {
	case domain.LockReasonLevel:
		return apiscenario.NewLockReasonLevel(r.RequiredLevel, r.CurrentLevel)
	case domain.LockReasonFaction:
		return apiscenario.NewLockReasonFaction(r.RequiredFaction)
	case domain.LockReasonEpisode:
		return apiscenario.NewLockReasonEpisode(r.RequiredEpisode)
	default:
		return apiscenario.LockReason{Type: string(r.Type)}
	}
}

// ToLockReasons は domain.LockReason slice を wire LockReason slice に詰め替えます。
func ToLockReasons(reasons []domain.LockReason) []apiscenario.LockReason {
	if len(reasons) == 0 {
		return nil
	}
	result := make([]apiscenario.LockReason, len(reasons))
	for i, r := range reasons {
		result[i] = ToLockReason(r)
	}
	return result
}

// BuildEpisodeWithStatus は Episode と UnlockContext からアンロック状態と
// ロック理由を派生させて wire の EpisodeWithStatus を組み立てます。
func BuildEpisodeWithStatus(ep *domain.Episode, uc *domain.UnlockContext, lang string) apiscenario.EpisodeWithStatus {
	reasons := ep.LockReasons(uc)
	return apiscenario.EpisodeWithStatus{
		EpisodeID:     ep.EpisodeID,
		Faction:       ep.Faction,
		EpisodeNumber: ep.EpisodeNumber,
		Title:         ep.Title(lang),
		IsCompleted:   uc.CompletedEpisodes[ep.EpisodeID],
		IsUnlocked:    len(reasons) == 0,
		LockReasons:   ToLockReasons(reasons),
	}
}

// BuildEpisodesWithStatus は Episode slice を共通の UnlockContext に対して評価し、
// EpisodeWithStatus slice として返します。
func BuildEpisodesWithStatus(episodes []*domain.Episode, uc *domain.UnlockContext, lang string) []apiscenario.EpisodeWithStatus {
	result := make([]apiscenario.EpisodeWithStatus, len(episodes))
	for i, ep := range episodes {
		result[i] = BuildEpisodeWithStatus(ep, uc, lang)
	}
	return result
}
