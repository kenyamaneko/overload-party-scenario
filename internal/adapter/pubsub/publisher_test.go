package pubsub

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_Validation(t *testing.T) {
	tests := []struct {
		name                 string
		projectID            string
		playerOnboardedTopic string
		wantSubs             string
	}{
		{
			name:                 "projectID が空",
			projectID:            "",
			playerOnboardedTopic: "player-onboarded",
			wantSubs:             "projectID is empty",
		},
		{
			name:                 "player-onboarded topic 名が空",
			projectID:            "test-project",
			playerOnboardedTopic: "",
			wantSubs:             "playerOnboardedTopic is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := New(context.Background(), tt.projectID, tt.playerOnboardedTopic)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSubs)
			assert.Nil(t, p)
		})
	}
}

func TestPublish_UnknownEventType(t *testing.T) {
	p := &Publisher{}
	err := p.Publish(context.Background(), "unknown-event-type", []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown event type")
}
