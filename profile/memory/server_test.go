package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/profile/tests"
)

func TestProfile_MemoryServer(t *testing.T) {
	testStore := NewInMemory()
	teardown := func() {
		// testStore.(*memory).reset()
	}
	tests.RunServerTests(t, testStore, teardown)
}
