//go:build android

package android

import (
	"testing"

	"github.com/code-payments/flipchat-server/iap/tests"
)

func TestAndroidVerifier(t *testing.T) {
	// From real Android app on real environment.
	testPurchaseToken := "gcjkgkiehhchodpancdfjgfo.AO-J1OyEz6mLitFxK7gDOBN0iv4_9f5Xc6dIAdK_tLj2SGi9msJz-R5Xo3PcbC3fUYdG9SeQ6ngy2nwLe-LW2ORtPt6JQZte4w"

	// From test environment.
	//testPurchaseToken := "cmpkkdbgkebjhnalcgjinpba.AO-J1OzkqS9nR3iaT5C8C6HfVp_dqWvYoVjt8HACHXKDCXNioqPifcOxx3g33mZ36OAYqQvzxnUUX_YkNgRvlzSYQ7vBD6wRsQ"

	// TODO: Replace this with a real serviceAccount json.
	serviceAccount := []byte(`{
		"type": "service_account",
		"project_id": "flipchat-fd439",
		"private_key_id": "e9e710489780df977d4925d71f2460a37d7e2392",
		"private_key": "<REPLACE_ME>",
		"client_email": "flipchat-server@flipchat-fd439.iam.gserviceaccount.com",
		"client_id": "107283323988202137316",
		"auth_uri": "https://accounts.google.com/o/oauth2/auth",
		"token_uri": "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/flipchat-server%40flipchat-fd439.iam.gserviceaccount.com",
		"universe_domain": "googleapis.com"
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
