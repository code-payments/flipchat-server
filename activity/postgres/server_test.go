//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	account "github.com/code-payments/flipchat-server/account/postgres"
	"github.com/code-payments/flipchat-server/activity/tests"
	chat "github.com/code-payments/flipchat-server/chat/postgres"
	profile "github.com/code-payments/flipchat-server/profile/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestAccount_PostgresServer(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testEnv.DatabaseUrl)
	require.NoError(t, err)
	defer pool.Close()

	testStore := NewInPostgres(pool)
	accounts := account.NewInPostgres(pool)
	chats := chat.NewInPostgres(pool)
	profiles := profile.NewInPostgres(pool)
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunServerTests(t, accounts, testStore, chats, profiles, teardown)
}
