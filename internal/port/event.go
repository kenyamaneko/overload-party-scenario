package port

import "context"

// FactionPublisher は faction-selected イベントをイベントバスに発行する。
type FactionPublisher interface {
	// PublishFactionSelected は指定プレイヤーの faction 選択イベントを発行する。
	PublishFactionSelected(ctx context.Context, playerID, faction string) error
}
