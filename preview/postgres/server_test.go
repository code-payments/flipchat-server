package postgres

import (
	"testing"

	"go.uber.org/zap"

	prismatest "github.com/code-payments/flipchat-server/database/prisma/test"
	moderation "github.com/code-payments/flipchat-server/moderation/memory"

	"github.com/code-payments/flipchat-server/preview"
	"github.com/code-payments/flipchat-server/preview/tests"
)

func TestPreview_PostgresServer(t *testing.T) {
	client, disconnect := prismatest.NewTestClient(testEnv.DatabaseUrl, t)
	defer disconnect()

	previewStore := NewInPostgres(client)

	// Setup logger
	log := zap.NewNop()

	// Teardown function
	teardown := func() {
		previewStore.(*store).reset()
	}

	// Create server (safe)
	unflaggedModeration := moderation.NewClient(false) // Always safe
	server := preview.NewServer(log, previewStore, unflaggedModeration)
	tests.RunUnflaggedServerTests(t, server, previewStore, teardown)

	// Create server (flagged)
	flaggedModeration := moderation.NewClient(true) // Always flagged
	server = preview.NewServer(log, previewStore, flaggedModeration)
	tests.RunFlaggedServerTests(t, server, previewStore, teardown)
}
