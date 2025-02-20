package memory

import (
	"testing"

	account "github.com/code-payments/flipchat-server/account/memory"
	chat "github.com/code-payments/flipchat-server/chat/memory"
	"github.com/code-payments/flipchat-server/promoted/tests"
)

func TestPromoted_MemoryServer(t *testing.T) {
	accountStore := account.NewInMemory()
	chatStore := chat.NewInMemory()
	testStore := NewInMemory()
	teardown := func() {
		testStore.(*memoryStore).reset()
	}
	tests.RunServerTests(t, accountStore, chatStore, testStore, teardown)
}
