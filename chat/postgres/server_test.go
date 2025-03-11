//go:build integration

package postgres

import (
	"context"
	"testing"

	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"
	"github.com/stretchr/testify/require"

	account "github.com/code-payments/flipchat-server/account/postgres"
	intent "github.com/code-payments/flipchat-server/intent/postgres"
	messaging "github.com/code-payments/flipchat-server/messaging/memory"
	profile "github.com/code-payments/flipchat-server/profile/postgres"

	"github.com/code-payments/flipchat-server/chat/tests"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestChat_PostgresServer(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testEnv.DatabaseUrl)
	require.NoError(t, err)
	defer pool.Close()

	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	chats := NewInPostgres(pool)
	accounts := account.NewInPostgres(pool)
	profiles := profile.NewInPostgres(pool)
	intents := intent.NewInPostgres(client)
	messages := messaging.NewInMemory() // TODO: Implement Postgres messaging

	teardown := func() {
		chats.(*store).reset()
	}

	tests.RunServerTests(
		t, accounts, profiles, chats, messages, messages, intents, teardown)
}
