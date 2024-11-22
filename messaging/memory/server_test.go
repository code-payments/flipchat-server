package memory

import (
	"testing"

	account "github.com/code-payments/flipchat-server/account/memory"
	chat "github.com/code-payments/flipchat-server/chat/memory"
	"github.com/code-payments/flipchat-server/messaging/tests"
)

func TestMessaging_MemoryServer(t *testing.T) {
	accounts := account.NewInMemory()
	chats := chat.NewInMemory()
	testStore := NewInMemory()
	teardown := func() {
		//testStore.(*memory).reset()
	}
	tests.RunServerTests(t, accounts, testStore, testStore, chats, teardown)
}
