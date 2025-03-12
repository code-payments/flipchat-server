//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	account "github.com/code-payments/flipchat-server/account/postgres"
	intent "github.com/code-payments/flipchat-server/intent/postgres"
	messaging "github.com/code-payments/flipchat-server/messaging/postgres"
	profile "github.com/code-payments/flipchat-server/profile/postgres"

	"github.com/code-payments/flipchat-server/chat/tests"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestChat_PostgresServer(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testEnv.DatabaseUrl)
	require.NoError(t, err)
	defer pool.Close()

	chats := NewInPostgres(pool)
	accounts := account.NewInPostgres(pool)
	profiles := profile.NewInPostgres(pool)
	intents := intent.NewInPostgres(pool)
	messages := messaging.NewInPostgresMessages(pool)
	pointers := messaging.NewInPostgresPointers(pool)

	teardown := func() {
		chats.(*store).reset()
	}

	tests.RunServerTests(
		t, accounts, profiles, chats, messages, pointers, intents, teardown)
}
