package memory

import (
	"testing"

	account "github.com/code-payments/flipchat-server/account/memory"
	"github.com/code-payments/flipchat-server/messaging/tests"
)

func TestMessaging_MemoryServer(t *testing.T) {
	accounts := account.NewInMemory()
	testStore := NewInMemory()
	teardown := func() {
		//testStore.(*memory).reset()
	}
	tests.RunServerTests(t, accounts, testStore, testStore, teardown)
}
