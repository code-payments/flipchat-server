//go:build integration

package postgres

import (
	"context"
	"testing"

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

	pushes := NewInPostgres(pool)
	profiles := profile.NewInPostgres(pool)
	chats := chat.NewInPostgres(pool)

	teardown := func() {
		pushes.(*store).reset()
	}
	tests.RunMessagingTests(t, pushes, profiles, chats, teardown)
}
