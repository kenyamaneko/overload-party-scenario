package apiscenariofake_test

import (
	"context"
	"testing"
	"time"

	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
	"github.com/kenyamaneko/overload-party-scenario/packages/api-scenario/apiscenariofake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Expect → Publish → Wait の round-trip: scenario 送信側の typed publish と
// consumer 側の typed 受信が矛盾なく繋がる基本契約。consumer サービス
// (account / card / gateway) が subscribe テストを書く際の最小使用例でもある。
func TestPlayerOnboarded_ExpectPublishWaitRoundTrip(t *testing.T) {
	broker := apiscenariofake.NewBroker()
	pub := apiscenariofake.NewPublisher(broker)
	sub := apiscenariofake.NewSubscriber(broker)
	ctx := context.Background()

	exp := apiscenariofake.ExpectPlayerOnboarded(sub)

	require.NoError(t, apiscenariofake.PublishPlayerOnboarded(ctx, pub, apiscenario.PlayerOnboardedEvent{
		PlayerID:         "player-1",
		DisplayName:      "Kenya",
		InitialFactionID: "Tenki",
	}))

	got, err := exp.Wait(time.Second)
	require.NoError(t, err)
	assert.Equal(t, "player-1", got.PlayerID)
	assert.Equal(t, "Kenya", got.DisplayName)
	assert.Equal(t, "Tenki", got.InitialFactionID)
}

// PublishPlayerOnboarded は、caller が省略したフィールドに契約上のデフォルトを
// 埋める。テスト側は検証したい PlayerID / DisplayName / InitialFactionID のみ
// 書けばよい、という ergonomic の契約。
func TestPlayerOnboarded_PublishFillsDefaults(t *testing.T) {
	broker := apiscenariofake.NewBroker()
	pub := apiscenariofake.NewPublisher(broker)
	sub := apiscenariofake.NewSubscriber(broker)
	ctx := context.Background()

	before := time.Now().UTC()
	exp := apiscenariofake.ExpectPlayerOnboarded(sub)

	require.NoError(t, apiscenariofake.PublishPlayerOnboarded(ctx, pub, apiscenario.PlayerOnboardedEvent{
		PlayerID: "p", DisplayName: "n", InitialFactionID: "SHE",
	}))

	got, err := exp.Wait(time.Second)
	require.NoError(t, err)
	assert.Equal(t, apiscenario.EventTypePlayerOnboarded, got.EventType, "EventType は契約で固定")
	assert.NotEmpty(t, got.EventID, "EventID は未指定なら自動生成される")
	assert.False(t, got.Timestamp.Before(before), "Timestamp は未指定なら現在時刻以降")
}

// Expect で subscribe していない状態 (publish が先) では Wait が message を
// 拾わず timeout error を返す。consumer 側テストが「subscribe を忘れた」
// ケースを fake 側で黙って成功させない契約を fix する。
func TestPlayerOnboarded_WaitTimesOutWhenPublishedBeforeExpect(t *testing.T) {
	broker := apiscenariofake.NewBroker()
	pub := apiscenariofake.NewPublisher(broker)
	sub := apiscenariofake.NewSubscriber(broker)
	ctx := context.Background()

	require.NoError(t, apiscenariofake.PublishPlayerOnboarded(ctx, pub, apiscenario.PlayerOnboardedEvent{
		PlayerID: "p", DisplayName: "n", InitialFactionID: "Tenki",
	}))

	exp := apiscenariofake.ExpectPlayerOnboarded(sub)
	_, err := exp.Wait(50 * time.Millisecond)
	require.ErrorContains(t, err, "timeout")
}

// Publisher.Published() は typed helper 経由の publish でも同じ記録 API を提供する。
// 送信側サービスが「TopicPlayerOnboarded に正しく発行されたか」を低レベル API
// からでも確認できることで、typed helper の追加で観測性が落ちないことを固定する。
func TestTyped_PublishedRecordsTopicAndPayload(t *testing.T) {
	broker := apiscenariofake.NewBroker()
	pub := apiscenariofake.NewPublisher(broker)
	ctx := context.Background()

	require.NoError(t, apiscenariofake.PublishPlayerOnboarded(ctx, pub, apiscenario.PlayerOnboardedEvent{
		PlayerID: "p", DisplayName: "n", InitialFactionID: "Sugar",
	}))

	history := pub.Published()
	require.Len(t, history, 1)
	assert.Equal(t, apiscenario.TopicPlayerOnboarded, history[0].Topic)
	// payload は json bytes。decode テストは round-trip 系で既に担保済みのため
	// ここでは topic 配線が意図通りであることまで固定する。
	assert.NotEmpty(t, history[0].Data)
}
