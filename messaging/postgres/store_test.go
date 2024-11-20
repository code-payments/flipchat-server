//go:build integration

package postgres

import (
	"testing"

	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	"github.com/code-payments/flipchat-server/messaging/tests"

	_ "github.com/jackc/pgx/v4/stdlib"
)

func TestMessaging_PostgresStore(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	messages := NewInPostgresMessages(client)
	pointers := NewInPostgresPointers(client)

	teardown := func() {
		messages.(*store).reset()
	}

	tests.RunStoreTests(t, messages, pointers, teardown)
}
