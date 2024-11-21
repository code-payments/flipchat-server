package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/push/tests"
)

func TestPush_MemoryServer(t *testing.T) {
	pushes := NewInMemory()

	teardown := func() {
		// testStore.(*memory).reset()
	}
	tests.RunServerTests(t, pushes, teardown)
}
