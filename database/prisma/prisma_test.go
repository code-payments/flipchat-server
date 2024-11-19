//go:build integration

package database

import (
	"context"
	"os"
	"testing"

	"github.com/sirupsen/logrus"

	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	_ "github.com/jackc/pgx/v4/stdlib"
)

var testEnv *prismatest.TestEnv

func TestMain(m *testing.M) {
	log := logrus.StandardLogger()

	// Create a new test environment
	env, err := prismatest.NewTestEnv()
	if err != nil {
		log.WithError(err).Error("Error creating test environment")
		os.Exit(1)
	}

	// Set the test environment
	testEnv = env

	// Run tests
	code := m.Run()
	os.Exit(code)
}

func TestCheckForMigrations(t *testing.T) {
	client, cleanFn := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer cleanFn()

	// Here we check for the existence of the _prisma_migrations table using the
	// prisma client.

	ctx := context.Background()
	_, err := client.Prisma.ExecuteRaw("SELECT * FROM _prisma_migrations").Exec(ctx)
	if err != nil {
		t.Fatalf("Error checking for migrations: %v", err)
	}
}
