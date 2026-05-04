package presenter

import (
	"time"

	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// ToOnboardingNameSetEvent は与えられた eventID / playerID / name / timestamp から
// wire の OnboardingNameSetEvent を組み立てます。
// uuid 採番や時刻採取などの副作用は呼び出し側 (usecase) が担います。
func ToOnboardingNameSetEvent(eventID, playerID, name string, ts time.Time) apiscenario.OnboardingNameSetEvent {
	return apiscenario.OnboardingNameSetEvent{
		EventType: apiscenario.EventTypeOnboardingNameSet,
		EventID:   eventID,
		Timestamp: ts,
		PlayerID:  playerID,
		Name:      name,
	}
}

// ToOnboardingFactionSetEvent は wire の OnboardingFactionSetEvent を組み立てます。
func ToOnboardingFactionSetEvent(eventID, playerID, initialFactionID string, ts time.Time) apiscenario.OnboardingFactionSetEvent {
	return apiscenario.OnboardingFactionSetEvent{
		EventType:        apiscenario.EventTypeOnboardingFactionSet,
		EventID:          eventID,
		Timestamp:        ts,
		PlayerID:         playerID,
		InitialFactionID: initialFactionID,
	}
}

// ToPlayerOnboardedEvent は wire の PlayerOnboardedEvent を組み立てます。
func ToPlayerOnboardedEvent(eventID, playerID, initialFactionID string, ts time.Time) apiscenario.PlayerOnboardedEvent {
	return apiscenario.PlayerOnboardedEvent{
		EventType:        apiscenario.EventTypePlayerOnboarded,
		EventID:          eventID,
		Timestamp:        ts,
		PlayerID:         playerID,
		InitialFactionID: initialFactionID,
	}
}
