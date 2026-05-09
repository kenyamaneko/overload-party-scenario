package domain

import "time"

// OnboardingNameSetEvent はオンボード内 name 入力ステップ完了の業務事実。
// subscriber: account (players.name + onboarding_status='name_set' を 1 tx で更新)。
type OnboardingNameSetEvent struct {
	EventID   string
	Timestamp time.Time
	PlayerID  string
	Name      string
}

// OnboardingFactionSetEvent はオンボード内 faction 選択ステップ完了の業務事実。
// subscriber: account (players.selected_faction + player_factions INSERT + onboarding_status='faction_set' を 1 tx で更新)。
type OnboardingFactionSetEvent struct {
	EventID          string
	Timestamp        time.Time
	PlayerID         string
	InitialFactionID string
}

// PlayerOnboardedEvent はオンボーディングシナリオ読了の業務事実 (player ごと一度)。
// subscriber: account (onboarding_status='completed') / card (initial pack 配布) / gateway (WS 通知)。
type PlayerOnboardedEvent struct {
	EventID          string
	Timestamp        time.Time
	PlayerID         string
	InitialFactionID string
}
