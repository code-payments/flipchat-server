package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/chat/tests"
)

func TestChat_MemoryStore(t *testing.T) {
	testStore := NewInMemory()
	teardown := func() {
		//testStore.(*memory).reset()
	}
	tests.RunStoreTests(t, testStore, teardown)
}
