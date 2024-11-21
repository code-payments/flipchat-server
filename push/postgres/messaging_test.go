//go:build integration

package postgres

import (
	"testing"

	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	chat "github.com/code-payments/flipchat-server/chat/postgres"
	profile "github.com/code-payments/flipchat-server/profile/postgres"

	"github.com/code-payments/flipchat-server/push/tests"

	_ "github.com/jackc/pgx/v4/stdlib"
)

func TestPush_PostgresMessaging(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	pushes := NewInPostgres(client)
	profiles := profile.NewInPostgres(client)
	chats := chat.NewInPostgres(client)

	teardown := func() {
		pushes.(*store).reset()
	}
	tests.RunMessagingTests(t, pushes, profiles, chats, teardown)
}
