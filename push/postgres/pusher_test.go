//go:build integration

package postgres

import (
	"testing"

	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	"github.com/code-payments/flipchat-server/push/tests"

	_ "github.com/jackc/pgx/v4/stdlib"
)

func TestPush_PostgresPusher(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	testStore := NewInPostgres(client)
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunPusherTests(t, testStore, teardown)
}
