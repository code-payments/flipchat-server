//go:build integration

package postgres

import (
	"testing"

	account "github.com/code-payments/flipchat-server/account/postgres"
	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"
	s3 "github.com/code-payments/flipchat-server/s3/memory"

	"github.com/code-payments/flipchat-server/blob/tests"

	_ "github.com/jackc/pgx/v4/stdlib"
)

func TestMessaging_PostgresServer(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	accounts := account.NewInPostgres(client)
	blobs := NewInPostgres(client)
	s3s := s3.NewInMemory()

	teardown := func() {
		blobs.(*store).reset()
	}

	tests.RunBlobServerTests(t, accounts, blobs, s3s, teardown)
}
