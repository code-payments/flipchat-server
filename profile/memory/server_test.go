package memory

import (
	"testing"

	account_memory "github.com/code-payments/flipchat-server/account/memory"
	"github.com/code-payments/flipchat-server/profile/tests"
)

func TestProfile_MemoryServer(t *testing.T) {
	accounts := account_memory.NewInMemory()
	profiles := NewInMemory()
	teardown := func() {
	}
	tests.RunServerTests(t, accounts, profiles, teardown)
}
