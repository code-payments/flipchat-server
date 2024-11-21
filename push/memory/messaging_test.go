package memory

import (
	"testing"

	chat "github.com/code-payments/flipchat-server/chat/memory"
	profile "github.com/code-payments/flipchat-server/profile/memory"

	"github.com/code-payments/flipchat-server/push/tests"
)

func TestPush_MemoryMessaging(t *testing.T) {
	pushes := NewInMemory()
	profiles := profile.NewInMemory()
	chats := chat.NewInMemory()

	teardown := func() {
		// testStore.(*memory).reset()
	}
	tests.RunMessagingTests(t, pushes, profiles, chats, teardown)
}
