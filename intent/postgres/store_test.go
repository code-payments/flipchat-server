package postgres

import (
	"testing"

	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	"github.com/code-payments/flipchat-server/intent/tests"

	_ "github.com/jackc/pgx/v4/stdlib"
)

func TestIntent_PostgresStore(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	testStore := NewPostgres(client)
	teardown := func() {
		//testStore.(*store).reset()
	}
	tests.RunStoreTests(t, testStore, teardown)
}
