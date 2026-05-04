-- overload-party-scenario - PostgreSQL DDL (service-owned)
--
-- Scope (ADR-014):
--   scenario.scenario_episodes        - エピソードマスター
--   scenario.episode_required_factions - エピソード必須ファクション
--   scenario.player_story_progress    - プレイヤーのストーリー完了履歴
--
-- psqldef 互換。
-- Cross-schema reference（player_id -> account.players）は FK を張らない。

CREATE SCHEMA IF NOT EXISTS scenario;

-- =============================================================================
-- Schema-local helpers
-- =============================================================================

CREATE OR REPLACE FUNCTION scenario.update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- Story Scenarios (schema: scenario)
-- =============================================================================

CREATE TABLE scenario.scenario_episodes (
  episode_id        VARCHAR(50) NOT NULL,            -- エピソードID（例: she_ep1, final）
  category          VARCHAR(20) NOT NULL CHECK (category IN ('main', 'side', 'event')), -- エピソード種別 (main / side / event)
  faction           VARCHAR(20) CHECK (faction IS NULL OR faction IN ('SHE', 'Tenki', 'Sugar', 'Tuners', 'Neutral')), -- 所属陣営（NULL: 全陣営共通）
  episode_number    BIGINT NOT NULL,                 -- 陣営内の章番号
  title_ja          VARCHAR(200) NOT NULL,           -- 日本語タイトル
  title_en          VARCHAR(200) NOT NULL,           -- 英語タイトル
  required_level    BIGINT NOT NULL,                 -- アンロックに必要なレベル
  required_episodes TEXT[] NOT NULL,                 -- アンロックに必要な完了済みエピソード
  script_path       VARCHAR(500) NOT NULL,           -- スクリプトパステンプレート（{lang} を言語コードに置換）
  thumbnail_path    VARCHAR(500),                    -- サムネイル画像パス
  sort_order        BIGINT NOT NULL,                 -- 表示順
  is_active         BOOLEAN NOT NULL,                -- 公開フラグ
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(), -- 作成日時
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(), -- 更新日時
  PRIMARY KEY (episode_id)
);

CREATE INDEX idx_scenario_episodes_sort ON scenario.scenario_episodes(sort_order);
CREATE TRIGGER trg_scenario_episodes_updated_at BEFORE UPDATE ON scenario.scenario_episodes FOR EACH ROW EXECUTE FUNCTION scenario.update_updated_at();

CREATE TABLE scenario.episode_required_factions (
  episode_id  VARCHAR(50) NOT NULL REFERENCES scenario.scenario_episodes(episode_id) ON DELETE CASCADE, -- エピソード参照
  faction_id  VARCHAR(20) NOT NULL CHECK (faction_id IN ('SHE', 'Tenki', 'Sugar', 'Tuners', 'Neutral')), -- 必要陣営
  PRIMARY KEY (episode_id, faction_id)
);

CREATE TABLE scenario.player_story_progress (
  player_id    UUID NOT NULL, -- 所有プレイヤー (cross-schema reference to account.players; app-level integrity, not enforced by FK)
  episode_id   VARCHAR(50) NOT NULL REFERENCES scenario.scenario_episodes(episode_id) ON DELETE RESTRICT, -- 完了したエピソードID
  completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),   -- 完了日時
  PRIMARY KEY (player_id, episode_id)
);

-- =============================================================================
-- Onboarding (schema: scenario)
-- =============================================================================

-- scenario.player_onboarding はプレイヤーがオンボーディングシナリオを完了したか
-- 一度だけの完了を一意制約で保証するためのテーブル。2 回目の INSERT は一意違反で
-- 弾かれ、service 層が ErrAlreadyOnboarded に分類する。
-- display_name / initial_faction_id は outbox 経由で account / card / gateway に
-- 配信されるため、ここでは保持しない（identity の SSoT は account.players）。
CREATE TABLE scenario.player_onboarding (
  player_id    UUID        NOT NULL, -- 所有プレイヤー (cross-schema reference to account.players; app-level integrity, not enforced by FK)
  completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),   -- 完了日時
  PRIMARY KEY (player_id)
);

-- =============================================================================
-- Transactional Outbox
-- =============================================================================

-- scenario.outbox_events は Pub/Sub 発行を DB commit と atomic に揃えるための
-- outbox。ビジネス行の INSERT と同一トランザクションで 1 行 INSERT され、
-- 別 goroutine の worker が未 publish 行を claim して Pub/Sub に送出する。
-- これにより「DB commit は成功したが publish が失敗してイベントが失われる」
-- dual-write 問題を構造的に排除する (ADR-021)。
--
-- 参照: ARCHITECTURE.md §「scenario が Outbox を持つ理由」 (ADR-021 で更新)
--      shop.outbox_events と同一スキーマ (shop が先行採用済み)。
--
-- event_id は payload 内 eventId と一致し、再試行しても同じ値。subscriber 側の
-- 冪等性キー (processed_events / 複合 PK) と協調して at-least-once を担保する。
-- 配信済み行は削除しない (監査・障害調査用に保持)。
CREATE TABLE scenario.outbox_events (
  event_id          UUID         NOT NULL,                    -- payload 内 eventId と一致
  event_type        VARCHAR(100) NOT NULL,                    -- 論理イベント種別 (apiscenario.EventType*)。adapter が物理 topic に解決する
  payload           JSONB        NOT NULL,                    -- JSON Marshal 済みイベント本体
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),      -- enqueue 日時
  published_at      TIMESTAMPTZ,                              -- NULL = 未配信
  failure_count     INT          NOT NULL,                    -- 連続失敗回数
  last_error        TEXT,                                     -- 直近エラーメッセージ
  last_attempted_at TIMESTAMPTZ,                              -- 直近 publish 試行日時
  PRIMARY KEY (event_id)
);

-- 未配信行だけに効く partial index。publish 済み行が積み上がっても worker の
-- claim クエリ SELECT ... WHERE published_at IS NULL ORDER BY created_at が常に O(未配信数)。
CREATE INDEX idx_outbox_events_unpublished
  ON scenario.outbox_events (created_at)
  WHERE published_at IS NULL;
