//go:build integration

package postgres

import (
	"context"
	"testing"

	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"
	"github.com/stretchr/testify/require"

	chat "github.com/code-payments/flipchat-server/chat/postgres"
	profile "github.com/code-payments/flipchat-server/profile/postgres"

	"github.com/code-payments/flipchat-server/push/tests"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPush_PostgresMessaging(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testEnv.DatabaseUrl)
	require.NoError(t, err)
	defer pool.Close()

	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	pushes := NewInPostgres(client)
	profiles := profile.NewInPostgres(pool)
	chats := chat.NewInPostgres(pool)

	teardown := func() {
		pushes.(*store).reset()
	}
	tests.RunMessagingTests(t, pushes, profiles, chats, teardown)
}
