package postgres

import (
	"testing"

	postgrestest "github.com/code-payments/flipchat-server/database/postgres/test"

	"github.com/code-payments/flipchat-server/account/tests"

	_ "github.com/jackc/pgx/v4/stdlib"
)

func TestAccount_PostgresStore2(t *testing.T) {
	db, disconnect, err := postgrestest.WaitForConnection(testEnv.DatabaseUrl, false)
	if err != nil {
		t.Fatalf("Error connecting to database: %v", err)
	}
	defer disconnect()

	testStore := NewInPostgres2(db)
	teardown := func() {
		testStore.(*pgStore).reset()
	}
	tests.RunStoreTests(t, testStore, teardown)
}
