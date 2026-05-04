package presenter_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-scenario/internal/presenter"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

func TestToOnboardingNameSetEvent(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := presenter.ToOnboardingNameSetEvent("evt-1", "p1", "Kenya", ts)

	assert.Equal(t, apiscenario.EventTypeOnboardingNameSet, got.EventType)
	assert.Equal(t, "evt-1", got.EventID)
	assert.Equal(t, ts, got.Timestamp)
	assert.Equal(t, "p1", got.PlayerID)
	assert.Equal(t, "Kenya", got.Name)
}

func TestToOnboardingFactionSetEvent(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := presenter.ToOnboardingFactionSetEvent("evt-2", "p1", "SHE", ts)

	assert.Equal(t, apiscenario.EventTypeOnboardingFactionSet, got.EventType)
	assert.Equal(t, "evt-2", got.EventID)
	assert.Equal(t, ts, got.Timestamp)
	assert.Equal(t, "p1", got.PlayerID)
	assert.Equal(t, "SHE", got.InitialFactionID)
}

func TestToPlayerOnboardedEvent(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := presenter.ToPlayerOnboardedEvent("evt-3", "p1", "SHE", ts)

	assert.Equal(t, apiscenario.EventTypePlayerOnboarded, got.EventType)
	assert.Equal(t, "evt-3", got.EventID)
	assert.Equal(t, ts, got.Timestamp)
	assert.Equal(t, "p1", got.PlayerID)
	assert.Equal(t, "SHE", got.InitialFactionID)
}
