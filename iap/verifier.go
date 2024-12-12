package iap

import "context"

type Verifier interface {

	// VerifyReceipt takes a IAP receipt (for iOS/Android usually a
	// base64-encoded string, for memory a signed payload) and determines if it
	// is valid and has purchased the required feature.
	VerifyReceipt(ctx context.Context, receipt string) (bool, error)
}
