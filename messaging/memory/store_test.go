package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/messaging/tests"
)

func TestMessaging_MemoryStore(t *testing.T) {
	testStore := NewInMemory()
	teardown := func() {
		//testStore.(*memory).reset()
	}
	tests.RunStoreTests(t, testStore, testStore, teardown)
}
