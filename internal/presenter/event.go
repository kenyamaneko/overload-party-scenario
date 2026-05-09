package presenter

import (
	"encoding/json"
	"fmt"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// ToOnboardingNameSetWire は domain event を wire payload に詰め替えて event_type と marshal 済み bytes を返す。
// usecase は戻り値の eventType と payload をそのまま port.OutboxEvent に格納できる (apiscenario の import 不要)。
func ToOnboardingNameSetWire(ev domain.OnboardingNameSetEvent) (eventType string, payload []byte, err error) {
	wire := apiscenario.OnboardingNameSetEvent{
		EventType: apiscenario.EventTypeOnboardingNameSet,
		EventID:   ev.EventID,
		Timestamp: ev.Timestamp,
		PlayerID:  ev.PlayerID,
		Name:      ev.Name,
	}
	payload, err = json.Marshal(wire)
	if err != nil {
		return "", nil, fmt.Errorf("marshal OnboardingNameSetEvent: %w", err)
	}
	return apiscenario.EventTypeOnboardingNameSet, payload, nil
}

// ToOnboardingFactionSetWire は ToOnboardingNameSetWire の OnboardingFactionSetEvent 版。
func ToOnboardingFactionSetWire(ev domain.OnboardingFactionSetEvent) (eventType string, payload []byte, err error) {
	wire := apiscenario.OnboardingFactionSetEvent{
		EventType:        apiscenario.EventTypeOnboardingFactionSet,
		EventID:          ev.EventID,
		Timestamp:        ev.Timestamp,
		PlayerID:         ev.PlayerID,
		InitialFactionID: ev.InitialFactionID,
	}
	payload, err = json.Marshal(wire)
	if err != nil {
		return "", nil, fmt.Errorf("marshal OnboardingFactionSetEvent: %w", err)
	}
	return apiscenario.EventTypeOnboardingFactionSet, payload, nil
}

// ToPlayerOnboardedWire は ToOnboardingNameSetWire の PlayerOnboardedEvent 版。
func ToPlayerOnboardedWire(ev domain.PlayerOnboardedEvent) (eventType string, payload []byte, err error) {
	wire := apiscenario.PlayerOnboardedEvent{
		EventType:        apiscenario.EventTypePlayerOnboarded,
		EventID:          ev.EventID,
		Timestamp:        ev.Timestamp,
		PlayerID:         ev.PlayerID,
		InitialFactionID: ev.InitialFactionID,
	}
	payload, err = json.Marshal(wire)
	if err != nil {
		return "", nil, fmt.Errorf("marshal PlayerOnboardedEvent: %w", err)
	}
	return apiscenario.EventTypePlayerOnboarded, payload, nil
}
