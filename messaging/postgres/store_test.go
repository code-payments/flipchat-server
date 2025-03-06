//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/code-payments/flipchat-server/messaging/tests"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestMessaging_PostgresStore(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testEnv.DatabaseUrl)
	require.NoError(t, err)
	defer pool.Close()

	messages := NewInPostgresMessages(pool)
	pointers := NewInPostgresPointers(pool)

	teardown := func() {
		messages.(*store).reset()
	}

	tests.RunStoreTests(t, messages, pointers, teardown)
}
