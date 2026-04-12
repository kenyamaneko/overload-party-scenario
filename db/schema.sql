-- overload-party-scenario - PostgreSQL DDL (service-owned)
--
-- Scope (ADR-014):
--   scenario.scenario_episodes        - エピソードマスター
--   scenario.episode_required_factions - エピソード必須ファクション
--   scenario.player_story_progress    - プレイヤーのストーリー完了履歴
--
-- psqldef 互換。shared.update_updated_at() を先に作成しておくこと。
-- Cross-schema reference（player_id -> account.players）は FK を張らない。

CREATE SCHEMA IF NOT EXISTS scenario;

-- =============================================================================
-- Story Scenarios (schema: scenario)
-- =============================================================================

CREATE TABLE scenario.scenario_episodes (
  episode_id        VARCHAR(50) NOT NULL,            -- エピソードID（例: she_ep1, final）
  category          VARCHAR(20) NOT NULL DEFAULT 'main' CHECK (category IN ('main', 'side', 'event')), -- エピソード種別 (main / side / event)
  faction           VARCHAR(20) CHECK (faction IS NULL OR faction IN ('SHE', 'Tenki', 'Sugar', 'Tuners', 'Neutral')), -- 所属陣営（NULL: 全陣営共通）
  episode_number    BIGINT NOT NULL,                 -- 陣営内の章番号
  title_ja          VARCHAR(200) NOT NULL,           -- 日本語タイトル
  title_en          VARCHAR(200) NOT NULL,           -- 英語タイトル
  required_level    BIGINT NOT NULL DEFAULT 1,       -- アンロックに必要なレベル (Default: 1)
  required_episodes TEXT[] NOT NULL DEFAULT '{}',    -- アンロックに必要な完了済みエピソード
  script_path       VARCHAR(500) NOT NULL,           -- スクリプトパステンプレート（{lang} を言語コードに置換）
  thumbnail_path    VARCHAR(500),                    -- サムネイル画像パス
  sort_order        BIGINT NOT NULL,                 -- 表示順
  is_active         BOOLEAN NOT NULL DEFAULT true,   -- 公開フラグ (Default: true)
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(), -- 作成日時
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(), -- 更新日時
  PRIMARY KEY (episode_id)
);

CREATE INDEX idx_scenario_episodes_sort ON scenario.scenario_episodes(sort_order);
CREATE TRIGGER trg_scenario_episodes_updated_at BEFORE UPDATE ON scenario.scenario_episodes FOR EACH ROW EXECUTE FUNCTION shared.update_updated_at();

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
