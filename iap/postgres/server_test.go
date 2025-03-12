//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	account "github.com/code-payments/flipchat-server/account/postgres"
	iap_memory "github.com/code-payments/flipchat-server/iap/memory"
	"github.com/code-payments/flipchat-server/iap/tests"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestChat_PostgresServer(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testEnv.DatabaseUrl)
	require.NoError(t, err)
	defer pool.Close()

	pub, priv, err := iap_memory.GenerateKeyPair()
	if err != nil {
		t.Fatalf("error generating key pair: %v", err)
	}

	verifier := iap_memory.NewMemoryVerifier(pub)
	validReceiptFunc := func(msg string) string {
		return iap_memory.GenerateValidReceipt(priv, msg)
	}

	accounts := account.NewInPostgres(pool)
	iaps := NewInPostgres(pool)

	teardown := func() {
		iaps.(*store).reset()
	}

	tests.RunServerTests(t, accounts, iaps, verifier, validReceiptFunc, teardown)
}
