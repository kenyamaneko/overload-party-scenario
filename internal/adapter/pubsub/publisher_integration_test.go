//go:build integration

package pubsub

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/adapter/pubsub/pubsubtest"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

const emulatorMessageTimeout = 5 * time.Second

var sharedEmulator *pubsubtest.Emulator

func TestMain(m *testing.M) {
	ctx := context.Background()
	em, err := pubsubtest.StartEmulator(ctx, "scenario-test")
	if err != nil {
		log.Fatalf("start pubsub emulator: %v", err)
	}
	sharedEmulator = em

	code := m.Run()

	if cerr := em.Close(ctx); cerr != nil {
		log.Printf("close emulator: %v", cerr)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func newTestPublisher(t *testing.T) (*Publisher, map[string]string) {
	t.Helper()
	nameSetTopic := sharedEmulator.CreateTopic(t, "TST-ONBOARDING-NAME-SET")
	factionSetTopic := sharedEmulator.CreateTopic(t, "TST-ONBOARDING-FACTION-SET")
	onboardedTopic := sharedEmulator.CreateTopic(t, "TST-PLAYER-ONBOARDED")

	p, err := New(context.Background(), sharedEmulator.ProjectID(), nameSetTopic, factionSetTopic, onboardedTopic)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })

	return p, map[string]string{
		apiscenario.EventTypeOnboardingNameSet:    nameSetTopic,
		apiscenario.EventTypeOnboardingFactionSet: factionSetTopic,
		apiscenario.EventTypePlayerOnboarded:      onboardedTopic,
	}
}

func TestNewIntegration(t *testing.T) {
	t.Run("[配信]Publisher の構築", func(t *testing.T) {
		cases := []struct {
			name                      string
			projectID                 string
			onboardingNameSetTopic    string
			onboardingFactionSetTopic string
			playerOnboardedTopic      string
			wantErrSubstring          string
		}{
			{
				name:                      "プロジェクトIDが空文字のとき、エラーになる",
				projectID:                 "",
				onboardingNameSetTopic:    "TST-ONBOARDING-NAME-SET",
				onboardingFactionSetTopic: "TST-ONBOARDING-FACTION-SET",
				playerOnboardedTopic:      "TST-PLAYER-ONBOARDED",
				wantErrSubstring:          "projectID",
			},
			{
				name:                      "名前入力ステップ完了イベントのtopicが空文字のとき、エラーになる",
				projectID:                 "tst-project",
				onboardingNameSetTopic:    "",
				onboardingFactionSetTopic: "TST-ONBOARDING-FACTION-SET",
				playerOnboardedTopic:      "TST-PLAYER-ONBOARDED",
				wantErrSubstring:          "topic",
			},
			{
				name:                      "陣営選択ステップ完了イベントのtopicが空文字のとき、エラーになる",
				projectID:                 "tst-project",
				onboardingNameSetTopic:    "TST-ONBOARDING-NAME-SET",
				onboardingFactionSetTopic: "",
				playerOnboardedTopic:      "TST-PLAYER-ONBOARDED",
				wantErrSubstring:          "topic",
			},
			{
				name:                      "プレイヤーオンボード完了イベントのtopicが空文字のとき、エラーになる",
				projectID:                 "tst-project",
				onboardingNameSetTopic:    "TST-ONBOARDING-NAME-SET",
				onboardingFactionSetTopic: "TST-ONBOARDING-FACTION-SET",
				playerOnboardedTopic:      "",
				wantErrSubstring:          "topic",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				p, err := New(context.Background(), tc.projectID, tc.onboardingNameSetTopic, tc.onboardingFactionSetTopic, tc.playerOnboardedTopic)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrSubstring)
				assert.Nil(t, p)
			})
		}

		t.Run("全て有効な値のとき、構築が成功する", func(t *testing.T) {
			p, _ := newTestPublisher(t)
			assert.NotNil(t, p)
		})

		t.Run("認証情報が解決できないとき、エラーになる", func(t *testing.T) {
			t.Setenv("PUBSUB_EMULATOR_HOST", "")
			t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(t.TempDir(), "does-not-exist.json"))

			p, err := New(context.Background(), "tst-project", "TST-ONBOARDING-NAME-SET", "TST-ONBOARDING-FACTION-SET", "TST-PLAYER-ONBOARDED")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "pubsub: new pubsub client")
			assert.Nil(t, p)
		})
	})
}

func TestPublisherClose(t *testing.T) {
	t.Run("[配信]Publisherのクローズ", func(t *testing.T) {
		t.Run("構築済みのPublisherを閉じると、エラーにならない", func(t *testing.T) {
			nameSetTopic := sharedEmulator.CreateTopic(t, "TST-ONBOARDING-NAME-SET")
			factionSetTopic := sharedEmulator.CreateTopic(t, "TST-ONBOARDING-FACTION-SET")
			onboardedTopic := sharedEmulator.CreateTopic(t, "TST-PLAYER-ONBOARDED")
			p, err := New(context.Background(), sharedEmulator.ProjectID(), nameSetTopic, factionSetTopic, onboardedTopic)
			require.NoError(t, err)

			err = p.Close()

			assert.NoError(t, err)
		})
	})
}

func TestPublish(t *testing.T) {
	t.Run("[配信]イベントの配信", func(t *testing.T) {
		cases := []string{
			apiscenario.EventTypeOnboardingNameSet,
			apiscenario.EventTypeOnboardingFactionSet,
			apiscenario.EventTypePlayerOnboarded,
		}
		for _, eventType := range cases {
			t.Run(fmt.Sprintf("イベント種別が%sのとき、対応するtopicにメッセージが送出される", eventType), func(t *testing.T) {
				p, topicsByEventType := newTestPublisher(t)
				sub := sharedEmulator.Subscribe(t, topicsByEventType[eventType])

				ctx := context.Background()
				payload := []byte(eventType)
				require.NoError(t, p.Publish(ctx, eventType, payload))

				msg, err := sub.WaitForMessage(ctx, emulatorMessageTimeout)
				require.NoError(t, err)
				assert.Equal(t, payload, msg.Data)
			})
		}

		t.Run("未登録のイベント種別で配信しようとすると、エラーになり、その後の有効なイベント種別での配信は成功する", func(t *testing.T) {
			p, topicsByEventType := newTestPublisher(t)

			err := p.Publish(context.Background(), "TST-UNKNOWN-EVENT-TYPE", []byte(`{}`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown event type")

			sub := sharedEmulator.Subscribe(t, topicsByEventType[apiscenario.EventTypeOnboardingNameSet])
			payload := []byte("payload-after-unknown-event-type")
			require.NoError(t, p.Publish(context.Background(), apiscenario.EventTypeOnboardingNameSet, payload))
			msg, err := sub.WaitForMessage(context.Background(), emulatorMessageTimeout)
			require.NoError(t, err)
			assert.Equal(t, payload, msg.Data)
		})

		t.Run("存在しないtopicへ配信しようとすると、エラーになり、その後の他の有効なtopicへの配信は成功する", func(t *testing.T) {
			factionSetTopic := sharedEmulator.CreateTopic(t, "TST-ONBOARDING-FACTION-SET")
			onboardedTopic := sharedEmulator.CreateTopic(t, "TST-PLAYER-ONBOARDED")
			p, err := New(context.Background(), sharedEmulator.ProjectID(), "TST-NONEXISTENT-TOPIC", factionSetTopic, onboardedTopic)
			require.NoError(t, err)
			t.Cleanup(func() { _ = p.Close() })

			err = p.Publish(context.Background(), apiscenario.EventTypeOnboardingNameSet, []byte("payload"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "publish event_type=")

			sub := sharedEmulator.Subscribe(t, factionSetTopic)
			payload := []byte("payload-after-nonexistent-topic")
			require.NoError(t, p.Publish(context.Background(), apiscenario.EventTypeOnboardingFactionSet, payload))
			msg, err := sub.WaitForMessage(context.Background(), emulatorMessageTimeout)
			require.NoError(t, err)
			assert.Equal(t, payload, msg.Data)
		})
	})
}
