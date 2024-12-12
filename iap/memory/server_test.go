package memory

import (
	"testing"

	account "github.com/code-payments/flipchat-server/account/memory"
	"github.com/code-payments/flipchat-server/iap/tests"
)

func TestIAP_MemoryServer(t *testing.T) {
	accounts := account.NewInMemory()

	// Provide a teardown function if necessary. Here it's no-op.
	teardown := func() {}

	tests.RunServerTests(t, accounts, teardown)
}
