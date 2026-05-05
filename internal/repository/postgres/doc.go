// Package postgres は port.*Repo / port.OutboxStore の PostgreSQL 実装を提供する。
// pgx の positional Scan で domain 型へ直接読み書きし、ビジネス行と outbox 行を
// 同一 tx で書く。
package postgres
