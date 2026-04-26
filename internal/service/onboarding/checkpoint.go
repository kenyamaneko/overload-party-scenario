package onboarding

// Checkpoint はオンボーディング再開時の次の演出位置を表す。
// 値は account の Player 状態 (Name / SelectedFaction の nullable) と
// scenario.player_onboarding の完了マーク存在から導出される派生値であり、
// scenario 側で永続化しない。
type Checkpoint string

const (
	// CheckpointStarted はシナリオ冒頭から再生する状態。account.Player.Name == nil。
	CheckpointStarted Checkpoint = "started"
	// CheckpointNameSet は名前確定後・初期 faction 選択前。
	// 名前入力ダイアログをスキップして faction 選択から再生する。
	CheckpointNameSet Checkpoint = "name_set"
	// CheckpointFactionSet は初期 faction 選択後・完了マーク前。最終演出から再生する。
	CheckpointFactionSet Checkpoint = "faction_set"
	// CheckpointCompleted は scenario.player_onboarding に完了マークありの状態。
	// クライアントはホームへ直行する。
	CheckpointCompleted Checkpoint = "completed"
)
