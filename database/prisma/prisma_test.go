package database

import (
	"context"
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/sirupsen/logrus"

	postgrestest "github.com/code-payments/flipchat-server/database/postgres/test"
	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	"github.com/code-payments/flipchat-server/database/prisma/db"

	_ "github.com/jackc/pgx/v4/stdlib"
)

var (
	testPool    *dockertest.Pool
	databaseUrl string
	cleanUpFunc func()
)

func TestMain(m *testing.M) {
	log := logrus.StandardLogger()

	var err error
	testPool, err = dockertest.NewPool("")
	if err != nil {
		log.WithError(err).Error("Error creating docker pool")
		os.Exit(1)
	}

	// Start a postgres container
	databaseUrl, err = postgrestest.StartPostgresDB(testPool)
	if err != nil {
		log.WithError(err).Error("Error starting postgres image")
		os.Exit(1)
	}

	_, cleanUpFunc, err = postgrestest.WaitForConnection(databaseUrl, true)
	if err != nil {
		log.WithError(err).Error("Error waiting for connection")
		os.Exit(1)
	}

	// Run tests
	code := m.Run()
	os.Exit(code)
}

func TestPrismaMigrateDeploy(t *testing.T) {
	err := prismatest.RunPrismaMigrateDeploy(databaseUrl)
	if err != nil {
		t.Fatalf("Error running prisma migrate deploy: %v", err)
	}
}

func TestCheckForMigrations(t *testing.T) {
	// Bug: the following does not work, the Query Engine still looks up the
	// .env file regardless of our settings here.

	// client = db.NewClient(db.WithDatasourceURL(databaseUrl))

	// This also does not work? So no way to pre-create the client inside the
	// RunTests() function?

	// os.Setenv("DATABASE_URL", databaseUrl)

	// Super annoying, but we need to set the DATABASE_URL env var here.
	t.Setenv("DATABASE_URL", databaseUrl)

	client := db.NewClient()
	if err := client.Prisma.Connect(); err != nil {
		t.Fatalf("Error connecting to Prisma client: %v", err)
	}

	defer func() {
		if err := client.Prisma.Disconnect(); err != nil {
			t.Fatalf("Error disconnecting from Prisma client: %v", err)
		}
	}()

	// Here we check for the existence of the _prisma_migrations table using the
	// prisma client.

	ctx := context.Background()
	_, err := client.Prisma.ExecuteRaw("SELECT * FROM _prisma_migrations").Exec(ctx)
	if err != nil {
		t.Fatalf("Error checking for migrations: %v", err)
	}
}
