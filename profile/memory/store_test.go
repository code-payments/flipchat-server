package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/profile/tests"
)

func TestProfile_MemoryStore(t *testing.T) {
	testStore := NewInMemory()
	teardown := func() {
		// testStore.(*memory).reset()
	}
	tests.RunStoreTests(t, testStore, teardown)
}
