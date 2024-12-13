package memory

import (
	"testing"

	account "github.com/code-payments/flipchat-server/account/memory"
	"github.com/code-payments/flipchat-server/iap/tests"
)

func TestIAP_MemoryServer(t *testing.T) {
	pub, priv, err := generateKeyPair()
	if err != nil {
		t.Fatalf("error generating key pair: %v", err)
	}

	verifier := NewMemoryVerifier(pub)
	validReceiptFunc := func(msg string) string {
		return generateValidReceipt(priv, msg)
	}

	accounts := account.NewInMemory()

	// Provide a teardown function if necessary. Here it's no-op.
	teardown := func() {}

	tests.RunServerTests(t, accounts, verifier, validReceiptFunc, teardown)
}
