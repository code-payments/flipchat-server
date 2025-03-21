package memory

import (
	"testing"

	account "github.com/code-payments/flipchat-server/account/memory"
	"github.com/code-payments/flipchat-server/activity/tests"
)

func TestActivity_MemoryServer(t *testing.T) {
	testStore := NewInMemory()
	accounts := account.NewInMemory()
	teardown := func() {
		testStore.(*InMemoryStore).reset()
	}
	tests.RunServerTests(t, accounts, testStore, teardown)
}
