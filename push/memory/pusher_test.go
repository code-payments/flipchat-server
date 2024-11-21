package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/push/tests"
)

func TestPush_MemoryPusher(t *testing.T) {
	pushes := NewInMemory()

	teardown := func() {
		// testStore.(*memory).reset()
	}
	tests.RunPusherTests(t, pushes, teardown)
}
