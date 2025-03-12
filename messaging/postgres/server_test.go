//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	account "github.com/code-payments/flipchat-server/account/postgres"
	chat "github.com/code-payments/flipchat-server/chat/postgres"
	intent "github.com/code-payments/flipchat-server/intent/postgres"

	"github.com/code-payments/flipchat-server/messaging/tests"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestMessaging_PostgresServer(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testEnv.DatabaseUrl)
	require.NoError(t, err)
	defer pool.Close()

	accounts := account.NewInPostgres(pool)
	chats := chat.NewInPostgres(pool)
	intents := intent.NewInPostgres(pool)
	messages := NewInPostgresMessages(pool)
	pointers := NewInPostgresPointers(pool)

	teardown := func() {
		messages.(*store).reset()
	}

	tests.RunServerTests(t, accounts, intents, messages, pointers, chats, teardown)
}
