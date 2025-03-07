//go:build integration

package postgres

import (
	"context"
	"testing"

	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"
	"github.com/stretchr/testify/require"

	account_postgres "github.com/code-payments/flipchat-server/account/postgres"
	chat_postgres "github.com/code-payments/flipchat-server/chat/postgres"
	"github.com/code-payments/flipchat-server/profile/tests"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestProfile_PostgresServer(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testEnv.DatabaseUrl)
	require.NoError(t, err)
	defer pool.Close()

	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	accounts := account_postgres.NewInPostgres(pool)
	chats := chat_postgres.NewInPostgres(client)
	profiles := NewInPostgres(pool)
	teardown := func() {
		profiles.(*store).reset()
	}
	tests.RunServerTests(t, accounts, chats, profiles, teardown)
}
