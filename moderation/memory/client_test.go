package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/moderation/tests"
)

func TestMemoryClient(t *testing.T) {
	client := NewClient(true) // Always flagged
	tests.RunFlaggedModerationTests(t, client, func() {
		// No teardown logic needed
	})

	client = NewClient(false) // Never flagged
	tests.RunUnflaggedModerationTests(t, client, func() {
		// No teardown logic needed
	})
}
