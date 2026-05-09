package presenter_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	"github.com/kenyamaneko/overload-party-scenario/internal/presenter"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

func TestToOnboardingNameSetWire(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	eventType, payload, err := presenter.ToOnboardingNameSetWire(domain.OnboardingNameSetEvent{
		EventID:   "evt-1",
		Timestamp: ts,
		PlayerID:  "p1",
		Name:      "Kenya",
	})
	require.NoError(t, err)
	assert.Equal(t, apiscenario.EventTypeOnboardingNameSet, eventType)

	var got apiscenario.OnboardingNameSetEvent
	require.NoError(t, json.Unmarshal(payload, &got))
	assert.Equal(t, apiscenario.EventTypeOnboardingNameSet, got.EventType)
	assert.Equal(t, "evt-1", got.EventID)
	assert.Equal(t, ts, got.Timestamp)
	assert.Equal(t, "p1", got.PlayerID)
	assert.Equal(t, "Kenya", got.Name)
}

func TestToOnboardingFactionSetWire(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	eventType, payload, err := presenter.ToOnboardingFactionSetWire(domain.OnboardingFactionSetEvent{
		EventID:          "evt-2",
		Timestamp:        ts,
		PlayerID:         "p1",
		InitialFactionID: "SHE",
	})
	require.NoError(t, err)
	assert.Equal(t, apiscenario.EventTypeOnboardingFactionSet, eventType)

	var got apiscenario.OnboardingFactionSetEvent
	require.NoError(t, json.Unmarshal(payload, &got))
	assert.Equal(t, apiscenario.EventTypeOnboardingFactionSet, got.EventType)
	assert.Equal(t, "evt-2", got.EventID)
	assert.Equal(t, ts, got.Timestamp)
	assert.Equal(t, "p1", got.PlayerID)
	assert.Equal(t, "SHE", got.InitialFactionID)
}

func TestToPlayerOnboardedWire(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	eventType, payload, err := presenter.ToPlayerOnboardedWire(domain.PlayerOnboardedEvent{
		EventID:          "evt-3",
		Timestamp:        ts,
		PlayerID:         "p1",
		InitialFactionID: "SHE",
	})
	require.NoError(t, err)
	assert.Equal(t, apiscenario.EventTypePlayerOnboarded, eventType)

	var got apiscenario.PlayerOnboardedEvent
	require.NoError(t, json.Unmarshal(payload, &got))
	assert.Equal(t, apiscenario.EventTypePlayerOnboarded, got.EventType)
	assert.Equal(t, "evt-3", got.EventID)
	assert.Equal(t, ts, got.Timestamp)
	assert.Equal(t, "p1", got.PlayerID)
	assert.Equal(t, "SHE", got.InitialFactionID)
}
