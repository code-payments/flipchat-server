//go:build integration

package postgres

import (
	"testing"

	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	account "github.com/code-payments/flipchat-server/account/postgres"
	intent "github.com/code-payments/flipchat-server/intent/postgres"
	profile "github.com/code-payments/flipchat-server/profile/postgres"

	"github.com/code-payments/flipchat-server/chat/tests"
	"github.com/code-payments/flipchat-server/messaging"

	_ "github.com/jackc/pgx/v4/stdlib"
)

func TestChat_PostgresServer(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	chats := NewPostgres(client)
	accounts := account.NewPostgres(client)
	profiles := profile.NewPostgres(client)
	intents := intent.NewPostgres(client)
	messages := messaging.NewMemory() // TODO: Implement Postgres messaging

	teardown := func() {
		chats.(*store).reset()
	}

	tests.RunServerTests(
		t, accounts, profiles, chats, messages, messages, intents, teardown)
}
