package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/activity/tests"
)

func TestActivity_MemoryStore(t *testing.T) {
	testStore := NewInMemory()
	teardown := func() {
		testStore.(*InMemoryStore).reset()
	}
	tests.RunStoreTests(t, testStore, teardown)
}
