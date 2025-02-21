package memory

import (
	"testing"

	chat "github.com/code-payments/flipchat-server/chat/memory"
	"github.com/code-payments/flipchat-server/promoted/tests"
)

func TestPromoted_MemoryStore(t *testing.T) {
	chatStore := chat.NewInMemory()
	promotedStore := NewInMemory()

	teardown := func() {
		promotedStore.(*memoryStore).reset()
	}
	tests.RunStoreTests(t, chatStore, promotedStore, teardown)
}
