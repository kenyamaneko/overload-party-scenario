package domain

import "time"

// Episode は scenario_episodes テーブル 1 行に対応する domain 表現。
// API 応答の EpisodeWithStatus は usecase で Episode + UnlockContext から組み立てる。
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
// players + player_factions + player_story_progress を集約した値オブジェクト。
type UnlockContext struct {
	PlayerLevel       int64
	OwnedFactions     map[string]bool
	CompletedEpisodes map[string]bool
}
