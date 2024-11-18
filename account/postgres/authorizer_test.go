package postgres

import (
	"testing"

	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	"github.com/code-payments/flipchat-server/account/tests"

	_ "github.com/jackc/pgx/v4/stdlib"
)

func TestAccount_PostgresAuthorizer(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(databaseUrl, t)
	defer disconnect()

	testStore := NewPostgres(client)
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunAuthorizerTests(t, testStore, teardown)
}
