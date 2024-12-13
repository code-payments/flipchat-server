//go:build integration

package postgres

import (
	"testing"

	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	account_postgres "github.com/code-payments/flipchat-server/account/postgres"
	"github.com/code-payments/flipchat-server/profile/tests"

	_ "github.com/jackc/pgx/v4/stdlib"
)

func TestProfile_PostgresServer(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	accounts := account_postgres.NewInPostgres(client)
	profiles := NewInPostgres(client)
	teardown := func() {
		profiles.(*store).reset()
	}
	tests.RunServerTests(t, accounts, profiles, teardown)
}
