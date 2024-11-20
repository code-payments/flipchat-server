//go:build integration

package postgres

import (
	"testing"

	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	account "github.com/code-payments/flipchat-server/account/postgres"
	intent "github.com/code-payments/flipchat-server/intent/postgres"
	messaging "github.com/code-payments/flipchat-server/messaging/memory"
	profile "github.com/code-payments/flipchat-server/profile/postgres"

	"github.com/code-payments/flipchat-server/chat/tests"

	_ "github.com/jackc/pgx/v4/stdlib"
)

func TestChat_PostgresServer(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	chats := NewInPostgres(client)
	accounts := account.NewInPostgres(client)
	profiles := profile.NewInPostgres(client)
	intents := intent.NewInPostgres(client)
	messages := messaging.NewInMemory() // TODO: Implement Postgres messaging

	teardown := func() {
		chats.(*store).reset()
	}

	tests.RunServerTests(
		t, accounts, profiles, chats, messages, messages, intents, teardown)
}
