package postgres

import (
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/sirupsen/logrus"

	postgrestest "github.com/code-payments/flipchat-server/database/postgres/test"
	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"

	"github.com/code-payments/flipchat-server/account/tests"

	_ "github.com/jackc/pgx/v4/stdlib"
)

var (
	testPool    *dockertest.Pool
	databaseUrl string
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

	// Wait for the database to be ready
	_, _, err = postgrestest.WaitForConnection(databaseUrl, true)
	if err != nil {
		log.WithError(err).Error("Error waiting for connection")
		os.Exit(1)
	}

	// Apply sql migrations
	err = prismatest.RunPrismaMigrateDeploy(databaseUrl)
	if err != nil {
		log.WithError(err).Error("Error running prisma migrate deploy")
		os.Exit(1)
	}

	// Run tests
	code := m.Run()
	os.Exit(code)
}

func TestAccount_Postgres(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(databaseUrl, t)
	defer disconnect()

	testStore := NewPostgres(client)
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunTests(t, testStore, teardown)
}
