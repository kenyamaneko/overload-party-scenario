-- Test-only stub for account-owned tables referenced cross-schema by scenario.
--
-- scenario.StoryRepository.GetUnlockContext は player_progression.level と player_factions を
-- 読むが、これらは ADR-014 上 account service 所有で scenario の db/schema.sql には
-- 含まれない。現行実装は「暫定」として unqualified 参照しているため、
-- search_path=scenario の下で解決される位置（= scenario schema 内）に最小定義を
-- テスト用に置く。account 実体に倣い level は player_progression に分離する。
--
-- 本スタブは production DB には適用されない（docker-entrypoint-initdb.d に入れない）。

CREATE TABLE IF NOT EXISTS scenario.players (
  player_id UUID PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS scenario.player_progression (
  player_id UUID PRIMARY KEY REFERENCES scenario.players(player_id) ON DELETE CASCADE,
  level     BIGINT NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS scenario.player_factions (
  player_id UUID        NOT NULL REFERENCES scenario.players(player_id) ON DELETE CASCADE,
  faction   VARCHAR(20) NOT NULL,
  PRIMARY KEY (player_id, faction)
);
