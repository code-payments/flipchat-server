package apple

import (
	"testing"

	"github.com/code-payments/flipchat-server/iap/apple/resources"
	"github.com/code-payments/flipchat-server/iap/tests"

	"github.com/devsisters/go-applereceipt"
	"github.com/devsisters/go-applereceipt/applepki"
)

func TestAppleVerifier(t *testing.T) {
	// This represents a mock base64-encoded PKCS#7 receipt. In a real environment,
	// the iOS app dev would provide you with a valid receipt from the device or sandbox.
	base64Receipt := resources.ValidAppleReceipt

	// Create an instance of the AppleVerifier, passing:
	// - AppleRootCAPem: the Apple root CA (placeholder)
	// - ExpectedBundleID: your app’s bundle identifier
	// - ExpectedVersion: your app’s current version
	// - DeviceHash: the device identifier’s SHA-1
	verifier := NewAppleVerifier(
		[]byte(resources.AppleRootCAPEM),
		"com.flipchat.app", // Bundle ID
		"1.0",              // App version
	)

	// The test harness requires a MessageGenerator function. For Apple receipts,
	// the concept of "message" doesn't strictly apply, so we provide a dummy function.
	messageGenerator := func() string {
		return "unused_in_apple_verifier"
	}

	// validReceiptFunc simulates returning the iOS app developer’s base64 receipt.
	// We simply return our placeholder base64Receipt. In production, you'd
	// replace this with a real receipt or inject it at test runtime.
	validReceiptFunc := func(_ string) string {
		return base64Receipt
	}

	// No-op teardown. Adjust if you have resources to clean up after tests.
	teardown := func() {}

	// Run the generic verifier tests. This calls:
	//   - testValidReceipt: which will use validReceiptFunc(...) to get a "valid" receipt.
	//   - testInvalidReceipt: which will use the literal "invalid" string.
	tests.RunGenericVerifierTests(t, verifier, messageGenerator, validReceiptFunc, teardown)
}

func TestAppleVerifier2(t *testing.T) {
	// This represents a mock base64-encoded PKCS#7 receipt. In a real environment,
	// the iOS app dev would provide you with a valid receipt from the device or sandbox.
	base64Receipt := resources.ValidAppleReceipt

	receipt, err := applereceipt.DecodeBase64(base64Receipt, applepki.CertPool())
	if err != nil {
		panic(err)
	}

	t.Logf("Receipt: %+v", receipt)
}
