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
// consumer 側の typed 受信が矛盾なく繋がる基本契約。
func TestOnboardingNameSet_ExpectPublishWaitRoundTrip(t *testing.T) {
	broker := apiscenariofake.NewBroker()
	pub := apiscenariofake.NewPublisher(broker)
	sub := apiscenariofake.NewSubscriber(broker)
	ctx := context.Background()

	exp := apiscenariofake.ExpectOnboardingNameSet(sub)

	require.NoError(t, apiscenariofake.PublishOnboardingNameSet(ctx, pub, apiscenario.OnboardingNameSetEvent{
		PlayerID: "player-1",
		Name:     "Kenya",
	}))

	got, err := exp.Wait(time.Second)
	require.NoError(t, err)
	assert.Equal(t, "player-1", got.PlayerID)
	assert.Equal(t, "Kenya", got.Name)
}

// PublishOnboardingNameSet は EventType / EventID / Timestamp を補完する。
func TestOnboardingNameSet_PublishFillsDefaults(t *testing.T) {
	broker := apiscenariofake.NewBroker()
	pub := apiscenariofake.NewPublisher(broker)
	sub := apiscenariofake.NewSubscriber(broker)
	ctx := context.Background()

	before := time.Now().UTC()
	exp := apiscenariofake.ExpectOnboardingNameSet(sub)

	require.NoError(t, apiscenariofake.PublishOnboardingNameSet(ctx, pub, apiscenario.OnboardingNameSetEvent{
		PlayerID: "p", Name: "n",
	}))

	got, err := exp.Wait(time.Second)
	require.NoError(t, err)
	assert.Equal(t, apiscenario.EventTypeOnboardingNameSet, got.EventType, "EventType は契約で固定")
	assert.NotEmpty(t, got.EventID, "EventID は未指定なら自動生成される")
	assert.False(t, got.Timestamp.Before(before), "Timestamp は未指定なら現在時刻以降")
}

// Expect より先に publish されたメッセージは Wait で拾えず timeout になる。
func TestOnboardingNameSet_WaitTimesOutWhenPublishedBeforeExpect(t *testing.T) {
	broker := apiscenariofake.NewBroker()
	pub := apiscenariofake.NewPublisher(broker)
	sub := apiscenariofake.NewSubscriber(broker)
	ctx := context.Background()

	require.NoError(t, apiscenariofake.PublishOnboardingNameSet(ctx, pub, apiscenario.OnboardingNameSetEvent{
		PlayerID: "p", Name: "n",
	}))

	exp := apiscenariofake.ExpectOnboardingNameSet(sub)
	_, err := exp.Wait(50 * time.Millisecond)
	require.ErrorContains(t, err, "timeout")
}

// OnboardingFactionSet 側の round-trip + defaults を固定する。
func TestOnboardingFactionSet_ExpectPublishWaitRoundTrip(t *testing.T) {
	broker := apiscenariofake.NewBroker()
	pub := apiscenariofake.NewPublisher(broker)
	sub := apiscenariofake.NewSubscriber(broker)
	ctx := context.Background()

	exp := apiscenariofake.ExpectOnboardingFactionSet(sub)

	require.NoError(t, apiscenariofake.PublishOnboardingFactionSet(ctx, pub, apiscenario.OnboardingFactionSetEvent{
		PlayerID:         "player-2",
		InitialFactionID: "SHE",
	}))

	got, err := exp.Wait(time.Second)
	require.NoError(t, err)
	assert.Equal(t, "player-2", got.PlayerID)
	assert.Equal(t, "SHE", got.InitialFactionID)
	assert.Equal(t, apiscenario.EventTypeOnboardingFactionSet, got.EventType, "EventType は契約で固定")
	assert.NotEmpty(t, got.EventID)
}

// PlayerOnboarded 側の round-trip + defaults を固定する。
func TestPlayerOnboarded_ExpectPublishWaitRoundTrip(t *testing.T) {
	broker := apiscenariofake.NewBroker()
	pub := apiscenariofake.NewPublisher(broker)
	sub := apiscenariofake.NewSubscriber(broker)
	ctx := context.Background()

	exp := apiscenariofake.ExpectPlayerOnboarded(sub)

	require.NoError(t, apiscenariofake.PublishPlayerOnboarded(ctx, pub, apiscenario.PlayerOnboardedEvent{
		PlayerID:         "player-3",
		InitialFactionID: "Tenki",
	}))

	got, err := exp.Wait(time.Second)
	require.NoError(t, err)
	assert.Equal(t, "player-3", got.PlayerID)
	assert.Equal(t, "Tenki", got.InitialFactionID)
	assert.Equal(t, apiscenario.EventTypePlayerOnboarded, got.EventType, "EventType は契約で固定")
	assert.NotEmpty(t, got.EventID)
}

// Publisher.Published() は typed helper 経由の publish でも記録に残る。
func TestTyped_PublishedRecordsTopicAndPayload(t *testing.T) {
	broker := apiscenariofake.NewBroker()
	pub := apiscenariofake.NewPublisher(broker)
	ctx := context.Background()

	require.NoError(t, apiscenariofake.PublishOnboardingNameSet(ctx, pub, apiscenario.OnboardingNameSetEvent{
		PlayerID: "p", Name: "n",
	}))
	require.NoError(t, apiscenariofake.PublishOnboardingFactionSet(ctx, pub, apiscenario.OnboardingFactionSetEvent{
		PlayerID: "p", InitialFactionID: "SHE",
	}))
	require.NoError(t, apiscenariofake.PublishPlayerOnboarded(ctx, pub, apiscenario.PlayerOnboardedEvent{
		PlayerID: "p", InitialFactionID: "SHE",
	}))

	history := pub.Published()
	require.Len(t, history, 3)
	assert.Equal(t, "onboarding-name-set", history[0].Topic)
	assert.Equal(t, "onboarding-faction-set", history[1].Topic)
	assert.Equal(t, "player-onboarded", history[2].Topic)
	assert.NotEmpty(t, history[0].Data)
	assert.NotEmpty(t, history[1].Data)
	assert.NotEmpty(t, history[2].Data)
}
