package memory

import (
	"testing"

	account "github.com/code-payments/flipchat-server/account/memory"
	chat "github.com/code-payments/flipchat-server/chat/memory"
	intent "github.com/code-payments/flipchat-server/intent/memory"
	"github.com/code-payments/flipchat-server/messaging/tests"
)

func TestMessaging_MemoryServer(t *testing.T) {
	accounts := account.NewInMemory()
	chats := chat.NewInMemory()
	intents := intent.NewInMemory()
	testStore := NewInMemory()
	teardown := func() {
		//testStore.(*memory).reset()
	}
	tests.RunServerTests(t, accounts, intents, testStore, testStore, chats, teardown)
}
