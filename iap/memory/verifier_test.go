package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/iap/tests"
)

func TestMemoryVerifier(t *testing.T) {
	pub, priv, err := generateKeyPair()
	if err != nil {
		t.Fatalf("error generating key pair: %v", err)
	}

	verifier := NewMemoryVerifier(pub)
	messageGenerator := func() string {
		return "paid_feature"
	}
	validReceiptFunc := func(msg string) string {
		return generateValidReceipt(priv, msg)
	}

	teardown := func() {}

	tests.RunGenericVerifierTests(t,
		verifier, messageGenerator, validReceiptFunc, teardown)
}
