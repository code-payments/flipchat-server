//go:build integration

package postgres

import (
	"testing"

	chat "github.com/code-payments/flipchat-server/chat/postgres"
	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	"github.com/code-payments/flipchat-server/promoted/tests"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPromoted_PostgresStore(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	chatStore := chat.NewInPostgres(client)
	testStore := NewInPostgres(client)
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunStoreTests(t, chatStore, testStore, teardown)
}
