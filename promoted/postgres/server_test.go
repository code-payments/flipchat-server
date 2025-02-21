//go:build integration

package postgres

import (
	"testing"

	account "github.com/code-payments/flipchat-server/account/postgres"
	chat "github.com/code-payments/flipchat-server/chat/postgres"
	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	"github.com/code-payments/flipchat-server/promoted/tests"

	_ "github.com/jackc/pgx/v4/stdlib"
)

func TestPromoted_PostgresServer(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	accountStore := account.NewInPostgres(client)
	chatStore := chat.NewInPostgres(client)
	testStore := NewInPostgres(client)
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunServerTests(t, accountStore, chatStore, testStore, teardown)
}
