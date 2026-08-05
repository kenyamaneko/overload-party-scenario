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

func TestOnboardingNameSet(t *testing.T) {
	t.Run("OnboardingNameSet typed helper", func(t *testing.T) {
		t.Run("Expect → Publish → Waitすると、typed publishとtyped受信が一致する", func(t *testing.T) {
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
		})

		t.Run("EventType / EventID / Timestampを指定しないとき、補完される", func(t *testing.T) {
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
		})

		t.Run("Expectより先にPublishしたとき、Waitがtimeoutする", func(t *testing.T) {
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
		})
	})
}

func TestOnboardingFactionSet(t *testing.T) {
	t.Run("OnboardingFactionSet typed helper", func(t *testing.T) {
		t.Run("Expect → Publish → Waitすると、typed publishとtyped受信が一致しEventType/EventIDが補完される", func(t *testing.T) {
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
		})
	})
}

func TestPlayerOnboarded(t *testing.T) {
	t.Run("PlayerOnboarded typed helper", func(t *testing.T) {
		t.Run("Expect → Publish → Waitすると、typed publishとtyped受信が一致しEventType/EventIDが補完される", func(t *testing.T) {
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
		})
	})
}

func TestTyped_PublishedRecordsTopicAndPayload(t *testing.T) {
	t.Run("typed helper経由のpublishの記録", func(t *testing.T) {
		t.Run("3種のtyped helperでpublishすると、Published()にtopic順で記録される", func(t *testing.T) {
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
		})
	})
}
