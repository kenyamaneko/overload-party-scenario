package pubsub

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// publisherTopics は test 内で New に渡す topic 名一式。
type publisherTopics struct {
	onboardingNameSet    string
	onboardingFactionSet string
	playerOnboarded      string
}

// loadTopicsFromEnv は production と同じ env 経由で topic 名を解決する。
// 個別 case で空文字を強制したい時のみ struct を上書きする。
func loadTopicsFromEnv(t *testing.T) publisherTopics {
	t.Helper()
	t.Setenv("ONBOARDING_NAME_SET_TOPIC", "onboarding-name-set")
	t.Setenv("ONBOARDING_FACTION_SET_TOPIC", "onboarding-faction-set")
	t.Setenv("PLAYER_ONBOARDED_TOPIC", "player-onboarded")
	return publisherTopics{
		onboardingNameSet:    os.Getenv("ONBOARDING_NAME_SET_TOPIC"),
		onboardingFactionSet: os.Getenv("ONBOARDING_FACTION_SET_TOPIC"),
		playerOnboarded:      os.Getenv("PLAYER_ONBOARDED_TOPIC"),
	}
}

// New の入力検証は gpubsub.NewClient 呼び出し前に return する。
func TestNew_Validation(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		override  func(topics *publisherTopics)
		wantSubs  string
	}{
		{
			name:      "projectID が空",
			projectID: "",
			override:  func(*publisherTopics) {},
			wantSubs:  "projectID is empty",
		},
		{
			name:      "onboarding-name-set topic 名が空",
			projectID: "test-project",
			override: func(topics *publisherTopics) {
				topics.onboardingNameSet = ""
			},
			wantSubs: "all topic names are required",
		},
		{
			name:      "onboarding-faction-set topic 名が空",
			projectID: "test-project",
			override: func(topics *publisherTopics) {
				topics.onboardingFactionSet = ""
			},
			wantSubs: "all topic names are required",
		},
		{
			name:      "player-onboarded topic 名が空",
			projectID: "test-project",
			override: func(topics *publisherTopics) {
				topics.playerOnboarded = ""
			},
			wantSubs: "all topic names are required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topics := loadTopicsFromEnv(t)
			tt.override(&topics)
			p, err := New(context.Background(), tt.projectID, topics.onboardingNameSet, topics.onboardingFactionSet, topics.playerOnboarded)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSubs)
			assert.Nil(t, p)
		})
	}
}

// Publish は未登録 eventType を Pub/Sub SDK に届く前に弾く。
func TestPublish_UnknownEventType(t *testing.T) {
	p := &Publisher{}
	err := p.Publish(context.Background(), "unknown-event-type", []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown event type")
}
