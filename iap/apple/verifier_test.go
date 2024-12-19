package apple

import (
	"testing"

	"github.com/code-payments/flipchat-server/iap/apple/resources"
	"github.com/code-payments/flipchat-server/iap/tests"
)

func TestAppleVerifier(t *testing.T) {
	// This represents a mock base64-encoded PKCS#7 receipt. In a real environment,
	// the iOS app dev would provide you with a valid receipt from the device or sandbox.
	base64Receipt := resources.ValidAppleReceipt

	verifier := NewAppleVerifier(
		"com.flipchat.app",
		"com.flipchat.iap.createAccount",
	)

	// The test harness requires a MessageGenerator function. For Apple receipts,
	// the concept of "message" doesn't strictly apply, so we provide a dummy function.
	messageGenerator := func() string {
		return "unused_in_apple_verifier"
	}

	// validReceiptFunc simulates returning the iOS app developer’s base64 receipt.
	// We simply return our placeholder base64Receipt.
	validReceiptFunc := func(_ string) string {
		return base64Receipt
	}

	// No-op teardown.
	teardown := func() {}

	tests.RunGenericVerifierTests(t, verifier, messageGenerator, validReceiptFunc, teardown)
}
