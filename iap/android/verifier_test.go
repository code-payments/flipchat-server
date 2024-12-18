//go:build integration

package android

import (
	"testing"

	"github.com/code-payments/flipchat-server/iap/tests"
)

func TestAppleVerifier(t *testing.T) {
	// From real Android app on real environment.
	testPurchaseToken := "gcjkgkiehhchodpancdfjgfo.AO-J1OyEz6mLitFxK7gDOBN0iv4_9f5Xc6dIAdK_tLj2SGi9msJz-R5Xo3PcbC3fUYdG9SeQ6ngy2nwLe-LW2ORtPt6JQZte4w"

	// From test environment.
	//testPurchaseToken := "cmpkkdbgkebjhnalcgjinpba.AO-J1OzkqS9nR3iaT5C8C6HfVp_dqWvYoVjt8HACHXKDCXNioqPifcOxx3g33mZ36OAYqQvzxnUUX_YkNgRvlzSYQ7vBD6wRsQ"

	// TODO: Replace this with a real serviceAccount json.
	serviceAccount := []byte(`{
		"type": "service_account",
		// ???
	}`)

	verifier := NewAndroidVerifier(
		serviceAccount,
		"xyz.flipchat.app",
		"com.flipchat.iap.createaccount",
	)

	// The test harness requires a MessageGenerator function. For Apple receipts,
	// the concept of "message" doesn't strictly apply, so we provide a dummy function.
	messageGenerator := func() string {
		return "unused_in_apple_verifier"
	}

	// validReceiptFunc simulates returning the iOS app developer’s base64 receipt.
	// We simply return our placeholder base64Receipt.
	validReceiptFunc := func(_ string) string {
		return testPurchaseToken
	}

	// No-op teardown.
	teardown := func() {}

	tests.RunGenericVerifierTests(t, verifier, messageGenerator, validReceiptFunc, teardown)
}
