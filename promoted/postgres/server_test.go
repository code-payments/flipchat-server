//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	account "github.com/code-payments/flipchat-server/account/postgres"
	chat "github.com/code-payments/flipchat-server/chat/postgres"
	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	"github.com/code-payments/flipchat-server/promoted/tests"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPromoted_PostgresServer(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testEnv.DatabaseUrl)
	require.NoError(t, err)
	defer pool.Close()

	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	accountStore := account.NewInPostgres(pool)
	chatStore := chat.NewInPostgres(client)
	testStore := NewInPostgres(client)
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunServerTests(t, accountStore, chatStore, testStore, teardown)
}
