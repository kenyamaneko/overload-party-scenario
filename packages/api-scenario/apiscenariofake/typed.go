package apiscenariofake

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// PublishPlayerOnboarded は scenario publisher の role を演じて
// TopicPlayerOnboarded へ PlayerOnboardedEvent を 1 件発行する。
// EventID / Timestamp が未設定なら UUIDv4 / 現在時刻を自動付与し、EventType は
// 常に EventTypePlayerOnboarded に固定する — テスト側で手書きする必要があるのは
// PlayerID / InitialFactionID など検証対象のフィールドのみ。
func PublishPlayerOnboarded(ctx context.Context, p *Publisher, ev apiscenario.PlayerOnboardedEvent) error {
	ev = fillPlayerOnboardedDefaults(ev)
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal PlayerOnboardedEvent: %w", err)
	}
	return p.Publish(ctx, apiscenario.TopicPlayerOnboarded, data)
}

// PlayerOnboardedExpecter は TopicPlayerOnboarded に subscribe 済みの待受器。
// ExpectPlayerOnboarded で subscribe を確定 → publish → Wait で型付き payload
// を受け取る順序を API レベルで強制することで、Broker が新規 subscriber に過去
// メッセージを配信しない制約 (実 Pub/Sub の subscription 新規作成挙動に揃える
// 意図) を破らない構造にしている。
type PlayerOnboardedExpecter struct {
	ch <-chan []byte
}

// ExpectPlayerOnboarded は TopicPlayerOnboarded に即時 subscribe し、
// Wait 可能な Expecter を返す。publish より前に呼び出す必要がある。
func ExpectPlayerOnboarded(s *Subscriber) *PlayerOnboardedExpecter {
	return &PlayerOnboardedExpecter{ch: s.Messages(apiscenario.TopicPlayerOnboarded)}
}

// Wait は Expecter が subscribe 開始した以降に publish された最初の
// PlayerOnboardedEvent を timeout 付きで取り出す。timeout 超過や
// payload デコード失敗は error で返し、zero 値 + error の契約とする。
func (e *PlayerOnboardedExpecter) Wait(timeout time.Duration) (apiscenario.PlayerOnboardedEvent, error) {
	var zero apiscenario.PlayerOnboardedEvent
	select {
	case data, ok := <-e.ch:
		if !ok {
			return zero, fmt.Errorf("channel closed for topic %q before receiving message", apiscenario.TopicPlayerOnboarded)
		}
		var v apiscenario.PlayerOnboardedEvent
		if err := json.Unmarshal(data, &v); err != nil {
			return zero, fmt.Errorf("unmarshal %q payload: %w", apiscenario.TopicPlayerOnboarded, err)
		}
		return v, nil
	case <-time.After(timeout):
		return zero, fmt.Errorf("timeout waiting for %q after %s", apiscenario.TopicPlayerOnboarded, timeout)
	}
}

// fillPlayerOnboardedDefaults は PlayerOnboardedEvent の定型フィールドを補完する。
// EventType は契約固定のため既存値に関わらず上書きし、EventID / Timestamp は
// caller が事前に意図的にセットした値があればそれを尊重する。
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
