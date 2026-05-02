package postgres_test

import (
	"os"
	"testing"

	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres/postgrestest"
)

var sharedPg *postgrestest.Postgres

// TestMain は scenario schema + account stub schema を適用した Postgres コンテナをパッケージ共有で起動する。
func TestMain(m *testing.M) {
	os.Exit(postgrestest.RunMain(m, &sharedPg,
		postgrestest.WithSchemaFile("db/schema.sql"),
		postgrestest.WithSchemaFile("internal/repository/postgres/testdata/account_stub.sql"),
		postgrestest.WithSchema("scenario"),
		postgrestest.WithSearchPath("scenario", "public"),
	))
}
