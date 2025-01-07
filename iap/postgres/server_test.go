//go:build integration

package postgres

import (
	"testing"

	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	account "github.com/code-payments/flipchat-server/account/postgres"
	iap_memory "github.com/code-payments/flipchat-server/iap/memory"
	"github.com/code-payments/flipchat-server/iap/tests"

	_ "github.com/jackc/pgx/v4/stdlib"
)

func TestChat_PostgresServer(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	pub, priv, err := iap_memory.GenerateKeyPair()
	if err != nil {
		t.Fatalf("error generating key pair: %v", err)
	}

	verifier := iap_memory.NewMemoryVerifier(pub)
	validReceiptFunc := func(msg string) string {
		return iap_memory.GenerateValidReceipt(priv, msg)
	}

	accounts := account.NewInPostgres(client)
	iaps := NewInPostgres(client)

	teardown := func() {
		iaps.(*store).reset()
	}

	tests.RunServerTests(t, accounts, iaps, verifier, validReceiptFunc, teardown)
}
