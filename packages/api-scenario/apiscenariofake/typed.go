package apiscenariofake

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// PublishOnboardingNameSet は OnboardingNameSet 論理チャネルへ
// OnboardingNameSetEvent を 1 件発行する。EventID / Timestamp 未設定なら
// UUIDv4 / 現在時刻を補完、EventType は常に上書きする。
func PublishOnboardingNameSet(ctx context.Context, p *Publisher, ev apiscenario.OnboardingNameSetEvent) error {
	ev = fillOnboardingNameSetDefaults(ev)
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal OnboardingNameSetEvent: %w", err)
	}
	return p.Publish(ctx, "onboarding-name-set", data)
}

// PublishOnboardingFactionSet は OnboardingFactionSet 論理チャネルへ
// OnboardingFactionSetEvent を 1 件発行する。
func PublishOnboardingFactionSet(ctx context.Context, p *Publisher, ev apiscenario.OnboardingFactionSetEvent) error {
	ev = fillOnboardingFactionSetDefaults(ev)
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal OnboardingFactionSetEvent: %w", err)
	}
	return p.Publish(ctx, "onboarding-faction-set", data)
}

// PublishPlayerOnboarded は PlayerOnboarded 論理チャネルへ
// PlayerOnboardedEvent を 1 件発行する。
func PublishPlayerOnboarded(ctx context.Context, p *Publisher, ev apiscenario.PlayerOnboardedEvent) error {
	ev = fillPlayerOnboardedDefaults(ev)
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal PlayerOnboardedEvent: %w", err)
	}
	return p.Publish(ctx, "player-onboarded", data)
}

// OnboardingNameSetExpecter は OnboardingNameSet に subscribe 済みの待受器。
// publish より前に subscribe を確定する必要があるため (Broker は過去メッセージを配信しない)、
// API は ExpectOnboardingNameSet → publish → Wait の順序を強制する形に分割している。
type OnboardingNameSetExpecter struct {
	ch <-chan []byte
}

// ExpectOnboardingNameSet は即時 subscribe して Expecter を返す (publish より前に呼ぶこと)。
func ExpectOnboardingNameSet(s *Subscriber) *OnboardingNameSetExpecter {
	return &OnboardingNameSetExpecter{ch: s.Messages("onboarding-name-set")}
}

// Wait は subscribe 後に publish された最初の event を timeout 付きで取り出す。
func (e *OnboardingNameSetExpecter) Wait(timeout time.Duration) (apiscenario.OnboardingNameSetEvent, error) {
	return waitTypedFromChan[apiscenario.OnboardingNameSetEvent](e.ch, "onboarding-name-set", timeout)
}

// OnboardingFactionSetExpecter は OnboardingFactionSet 版の Expecter。
type OnboardingFactionSetExpecter struct {
	ch <-chan []byte
}

// ExpectOnboardingFactionSet は OnboardingFactionSet に即時 subscribe し Expecter を返す。
func ExpectOnboardingFactionSet(s *Subscriber) *OnboardingFactionSetExpecter {
	return &OnboardingFactionSetExpecter{ch: s.Messages("onboarding-faction-set")}
}

// Wait は publish された最初の OnboardingFactionSetEvent を timeout 付きで取り出す。
func (e *OnboardingFactionSetExpecter) Wait(timeout time.Duration) (apiscenario.OnboardingFactionSetEvent, error) {
	return waitTypedFromChan[apiscenario.OnboardingFactionSetEvent](e.ch, "onboarding-faction-set", timeout)
}

// PlayerOnboardedExpecter は PlayerOnboarded 版の Expecter。
type PlayerOnboardedExpecter struct {
	ch <-chan []byte
}

// ExpectPlayerOnboarded は PlayerOnboarded に即時 subscribe し Expecter を返す。
func ExpectPlayerOnboarded(s *Subscriber) *PlayerOnboardedExpecter {
	return &PlayerOnboardedExpecter{ch: s.Messages("player-onboarded")}
}

// Wait は publish された最初の PlayerOnboardedEvent を timeout 付きで取り出す。
func (e *PlayerOnboardedExpecter) Wait(timeout time.Duration) (apiscenario.PlayerOnboardedEvent, error) {
	return waitTypedFromChan[apiscenario.PlayerOnboardedEvent](e.ch, "player-onboarded", timeout)
}

// waitTypedFromChan は payload bytes を timeout 付きで受信し型 T にデコードする。
func waitTypedFromChan[T any](ch <-chan []byte, topic string, timeout time.Duration) (T, error) {
	var zero T
	select {
	case data, ok := <-ch:
		if !ok {
			return zero, fmt.Errorf("channel closed for topic %q before receiving message", topic)
		}
		var v T
		if err := json.Unmarshal(data, &v); err != nil {
			return zero, fmt.Errorf("unmarshal %q payload: %w", topic, err)
		}
		return v, nil
	case <-time.After(timeout):
		return zero, fmt.Errorf("timeout waiting for %q after %s", topic, timeout)
	}
}

// fillOnboardingNameSetDefaults は EventType を上書きし、EventID / Timestamp 未設定なら補完する。
func fillOnboardingNameSetDefaults(ev apiscenario.OnboardingNameSetEvent) apiscenario.OnboardingNameSetEvent {
	ev.EventType = apiscenario.EventTypeOnboardingNameSet
	if ev.EventID == "" {
		ev.EventID = newEventID()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	return ev
}

// fillOnboardingFactionSetDefaults は OnboardingFactionSetEvent 版。
func fillOnboardingFactionSetDefaults(ev apiscenario.OnboardingFactionSetEvent) apiscenario.OnboardingFactionSetEvent {
	ev.EventType = apiscenario.EventTypeOnboardingFactionSet
	if ev.EventID == "" {
		ev.EventID = newEventID()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	return ev
}

// fillPlayerOnboardedDefaults は PlayerOnboardedEvent 版。
func fillPlayerOnboardedDefaults(ev apiscenario.PlayerOnboardedEvent) apiscenario.PlayerOnboardedEvent {
	ev.EventType = apiscenario.EventTypePlayerOnboarded
	if ev.EventID == "" {
		ev.EventID = newEventID()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	return ev
}
