package memory

import (
	"testing"

	"go.uber.org/zap"

	moderation "github.com/code-payments/flipchat-server/moderation/memory"
	"github.com/code-payments/flipchat-server/preview"
	"github.com/code-payments/flipchat-server/preview/tests"
)

func TestPreview_MemoryServer(t *testing.T) {
	// Initialize in-memory store
	store := NewInMemory()

	// Setup logger
	log := zap.NewNop()

	// Teardown function
	teardown := func() {
		store.(*memoryStore).reset()
	}

	// Create server (safe)
	unflaggedModeration := moderation.NewClient(false) // Always safe
	server := preview.NewServer(log, store, unflaggedModeration)
	tests.RunUnflaggedServerTests(t, server, store, teardown)

	// Create server (flagged)
	flaggedModeration := moderation.NewClient(true) // Always flagged
	server = preview.NewServer(log, store, flaggedModeration)
	tests.RunFlaggedServerTests(t, server, store, teardown)
}
