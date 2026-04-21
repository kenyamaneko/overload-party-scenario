package pubsub

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pubsubevents "github.com/kenyamaneko/overload-party-common/packages/pubsub-events"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

func TestNewEventBuilder_Validation(t *testing.T) {
	tests := []struct {
		name                 string
		factionSelectedTopic string
		playerOnboardedTopic string
		wantErr              bool
	}{
		{
			name:                 "両方空ならエラー",
			factionSelectedTopic: "",
			playerOnboardedTopic: "",
			wantErr:              true,
		},
		{
			name:                 "faction のみ空ならエラー",
			factionSelectedTopic: "",
			playerOnboardedTopic: "player-onboarded",
			wantErr:              true,
		},
		{
			name:                 "player-onboarded のみ空ならエラー",
			factionSelectedTopic: "faction-selected",
			playerOnboardedTopic: "",
			wantErr:              true,
		},
		{
			name:                 "両方埋まっていれば成功",
			factionSelectedTopic: "faction-selected",
			playerOnboardedTopic: "player-onboarded",
			wantErr:              false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := NewEventBuilder(tt.factionSelectedTopic, tt.playerOnboardedTopic)
			switch tt.wantErr {
			case true:
				require.Error(t, err)
				assert.Nil(t, b)
			case false:
				require.NoError(t, err)
				assert.NotNil(t, b)
			}
		})
	}
}

// BuildPlayerOnboarded は playerID / displayName / initialFactionID を検証した上で、
// 設定された player-onboarded topic と apiscenario.PlayerOnboardedEvent の shape に合う
// JSON payload を持つ OutboxEvent を返す。event_id は UUID で payload 内 eventId と一致する。
func TestBuildPlayerOnboarded(t *testing.T) {
	b, err := NewEventBuilder("faction-selected", "player-onboarded")
	require.NoError(t, err)

	t.Run("正常系", func(t *testing.T) {
		ev, err := b.BuildPlayerOnboarded("player-1", "テンキ太郎", "Tenki")
		require.NoError(t, err)

		assert.Equal(t, "player-onboarded", ev.Topic)
		assert.NotEqual(t, "", ev.EventID.String())

		var decoded apiscenario.PlayerOnboardedEvent
		require.NoError(t, json.Unmarshal(ev.Payload, &decoded))
		assert.Equal(t, apiscenario.EventTypePlayerOnboarded, decoded.EventType)
		assert.Equal(t, ev.EventID.String(), decoded.EventID, "payload 内 eventId は outbox 行の PK と一致する")
		assert.Equal(t, "player-1", decoded.PlayerID)
		assert.Equal(t, "テンキ太郎", decoded.DisplayName)
		assert.Equal(t, "Tenki", decoded.InitialFactionID)
		assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
	})

	t.Run("playerID が空ならエラー", func(t *testing.T) {
		_, err := b.BuildPlayerOnboarded("", "テンキ太郎", "Tenki")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "playerID is empty")
	})

	t.Run("displayName が空ならエラー", func(t *testing.T) {
		_, err := b.BuildPlayerOnboarded("player-1", "", "Tenki")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "displayName is empty")
	})

	t.Run("initialFactionID が空ならエラー", func(t *testing.T) {
		_, err := b.BuildPlayerOnboarded("player-1", "テンキ太郎", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "initialFactionID is empty")
	})
}

// BuildFactionSelected は scenario 起源の faction-selected イベントを構築する。
// Source は常に pubsubevents.FactionSourceScenarioInitial 固定 (shop 購入起因は別サービスが発行する)。
func TestBuildFactionSelected(t *testing.T) {
	b, err := NewEventBuilder("faction-selected", "player-onboarded")
	require.NoError(t, err)

	t.Run("正常系", func(t *testing.T) {
		ev, err := b.BuildFactionSelected("player-1", "Tenki")
		require.NoError(t, err)

		assert.Equal(t, "faction-selected", ev.Topic)
		assert.NotEqual(t, "", ev.EventID.String())

		var decoded pubsubevents.FactionSelectedEvent
		require.NoError(t, json.Unmarshal(ev.Payload, &decoded))
		assert.Equal(t, pubsubevents.EventTypeFactionSelected, decoded.EventType)
		assert.Equal(t, ev.EventID.String(), decoded.EventID, "payload 内 eventId は outbox 行の PK と一致する")
		assert.Equal(t, "player-1", decoded.PlayerID)
		assert.Equal(t, "Tenki", decoded.Faction)
		assert.Equal(t, pubsubevents.FactionSourceScenarioInitial, decoded.Source, "scenario 起源は常に FactionSourceScenarioInitial")
		assert.WithinDuration(t, time.Now(), decoded.Timestamp, 5*time.Second)
	})

	t.Run("playerID が空ならエラー", func(t *testing.T) {
		_, err := b.BuildFactionSelected("", "Tenki")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "playerID is empty")
	})

	t.Run("faction が空ならエラー", func(t *testing.T) {
		_, err := b.BuildFactionSelected("player-1", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "faction is empty")
	})
}

// event_id は毎回異なる UUID。subscriber 側の dedupe が正しく機能する前提 (同じ
// payload でも id が違えば新規イベントとして扱える) の素材になる。
func TestBuildPlayerOnboarded_EventIDUnique(t *testing.T) {
	b, err := NewEventBuilder("faction-selected", "player-onboarded")
	require.NoError(t, err)

	ev1, err := b.BuildPlayerOnboarded("player-1", "テンキ太郎", "Tenki")
	require.NoError(t, err)
	ev2, err := b.BuildPlayerOnboarded("player-1", "テンキ太郎", "Tenki")
	require.NoError(t, err)
	assert.NotEqual(t, ev1.EventID, ev2.EventID, "同一入力でも event_id は毎回異なる")
}
