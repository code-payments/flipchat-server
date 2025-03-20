package memory

import (
	"testing"

	account "github.com/code-payments/flipchat-server/account/memory"
	"github.com/code-payments/flipchat-server/activity/tests"
	chat "github.com/code-payments/flipchat-server/chat/memory"
	profile "github.com/code-payments/flipchat-server/profile/memory"
)

func TestActivity_MemoryServer(t *testing.T) {
	testStore := NewInMemory()
	accounts := account.NewInMemory()
	chats := chat.NewInMemory()
	profiles := profile.NewInMemory()
	teardown := func() {
		testStore.(*InMemoryStore).reset()
	}
	tests.RunServerTests(t, accounts, testStore, chats, profiles, teardown)
}
