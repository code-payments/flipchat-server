package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/iap/tests"
)

func TestMemoryVerifier(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("error generating key pair: %v", err)
	}

	verifier := NewMemoryVerifier(pub)
	messageGenerator := func() string {
		return "paid_feature"
	}
	validReceiptFunc := func(msg string) string {
		return GenerateValidReceipt(priv, msg)
	}

	teardown := func() {}

	tests.RunGenericVerifierTests(t,
		verifier, messageGenerator, validReceiptFunc, teardown)
}
