//go:build integration

package postgres

import (
	"testing"

	account "github.com/code-payments/flipchat-server/account/postgres"
	chat "github.com/code-payments/flipchat-server/chat/postgres"
	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"
	intent "github.com/code-payments/flipchat-server/intent/postgres"

	"github.com/code-payments/flipchat-server/messaging/tests"

	_ "github.com/jackc/pgx/v4/stdlib"
)

func TestMessaging_PostgresServer(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	accounts := account.NewInPostgres(client)
	chats := chat.NewInPostgres(client)
	intents := intent.NewInPostgres(client)
	messages := NewInPostgresMessages(client)
	pointers := NewInPostgresPointers(client)

	teardown := func() {
		messages.(*store).reset()
	}

	tests.RunServerTests(t, accounts, intents, messages, pointers, chats, teardown)
}
