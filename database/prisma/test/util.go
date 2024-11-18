package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ory/dockertest/v3"

	postgrestest "github.com/code-payments/flipchat-server/database/postgres/test"

	"github.com/code-payments/flipchat-server/database/prisma/db"
)

type TestEnv struct {
	TestPool    *dockertest.Pool
	DatabaseUrl string
}

func NewTestEnv() (*TestEnv, error) {
	var err error
	testPool, err := dockertest.NewPool("")
	if err != nil {
		return nil, err
	}

	// Start a postgres container
	databaseUrl, err := postgrestest.StartPostgresDB(testPool)
	if err != nil {
		return nil, err
	}

	// Wait for the database to be ready
	_, _, err = postgrestest.WaitForConnection(databaseUrl, true)
	if err != nil {
		return nil, err
	}

	// Apply sql migrations
	err = RunPrismaMigrateDeploy(databaseUrl)
	if err != nil {
		return nil, err
	}

	return &TestEnv{
		TestPool:    testPool,
		DatabaseUrl: databaseUrl,
	}, nil
}

// A bit of a hack, we should call the prisma migration programmatically
func RunPrismaMigrateDeploy(databaseUrl string) error {
	// Get the directory of the current file
	_, filePath, _, ok := runtime.Caller(0)
	if !ok {
		return os.ErrInvalid
	}

	prismaDir := filepath.Join(filepath.Dir(filePath), "../")
	if _, err := os.Stat(prismaDir); os.IsNotExist(err) {
		return err // prisma folder doesn't exist
	}

	cmd := exec.Command("go", "run", "github.com/steebchen/prisma-client-go", "migrate", "deploy")
	cmd.Env = append(os.Environ(), "DATABASE_URL="+databaseUrl)
	cmd.Dir = prismaDir // Set the working directory
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func NewTestClient(databaseUrl string, t *testing.T) (*db.PrismaClient, func()) {
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

	disconnectFn := func() {
		if err := client.Prisma.Disconnect(); err != nil {
			t.Fatalf("Error disconnecting from Prisma client: %v", err)
		}
	}

	return client, disconnectFn
}
