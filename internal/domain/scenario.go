package domain

import "time"

// Episode は scenario_episodes テーブル 1 行に対応する domain 表現。
// API 応答の EpisodeWithStatus は presenter で Episode + UnlockContext から組み立てる。
type Episode struct {
	EpisodeID        string
	Faction          *string
	EpisodeNumber    int64
	TitleJa          string
	TitleEn          string
	RequiredLevel    int64
	RequiredFactions []string
	RequiredEpisodes []string
	ScriptPath       string
	ThumbnailPath    *string
	SortOrder        int64
	IsActive         bool
	CreatedAt        time.Time
}

// UnlockContext はアンロック判定に必要なプレイヤー状態のスナップショット。
type UnlockContext struct {
	PlayerLevel       int64
	OwnedFactions     map[string]bool
	CompletedEpisodes map[string]bool
}

// LockReasonType はエピソードがロックされている理由の種別。
type LockReasonType string

const (
	LockReasonLevel   LockReasonType = "level"
	LockReasonFaction LockReasonType = "faction"
	LockReasonEpisode LockReasonType = "episode"
)

// LockReason はエピソードがロックされている個別理由を表す domain 値オブジェクト。
// wire DTO (apiscenario.LockReason) への射影は presenter が担う。
type LockReason struct {
	Type            LockReasonType
	RequiredLevel   int64
	CurrentLevel    int64
	RequiredFaction string
	RequiredEpisode string
}

// LockReasons は ep が uc 下で未充足の理由を全て返す。空 slice ならアンロック済み。
func (ep *Episode) LockReasons(uc *UnlockContext) []LockReason {
	var reasons []LockReason
	if uc.PlayerLevel < ep.RequiredLevel {
		reasons = append(reasons, LockReason{
			Type:          LockReasonLevel,
			RequiredLevel: ep.RequiredLevel,
			CurrentLevel:  uc.PlayerLevel,
		})
	}
	for _, f := range ep.RequiredFactions {
		if !uc.OwnedFactions[f] {
			reasons = append(reasons, LockReason{
				Type:            LockReasonFaction,
				RequiredFaction: f,
			})
		}
	}
	for _, reqEp := range ep.RequiredEpisodes {
		if !uc.CompletedEpisodes[reqEp] {
			reasons = append(reasons, LockReason{
				Type:            LockReasonEpisode,
				RequiredEpisode: reqEp,
			})
		}
	}
	return reasons
}

// Title は要求言語に応じたタイトルを返す。en 以外は ja を返す。
func (ep *Episode) Title(lang string) string {
	if lang == "en" {
		return ep.TitleEn
	}
	return ep.TitleJa
}
