package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/chat/tests"

	account "github.com/code-payments/flipchat-server/account/memory"
	intent "github.com/code-payments/flipchat-server/intent/memory"
	messaging "github.com/code-payments/flipchat-server/messaging"
	profile "github.com/code-payments/flipchat-server/profile/memory"
)

func TestChat_MemoryServer(t *testing.T) {
	chats := NewInMemory()
	accounts := account.NewInMemory()
	profiles := profile.NewInMemory()
	intents := intent.NewInMemory()
	messages := messaging.NewMemory()

	teardown := func() {
		//testStore.(*memory).reset()
	}

	tests.RunServerTests(
		t, accounts, profiles, chats, messages, messages, intents, teardown)
}
