package apple

import (
	"context"

	"github.com/devsisters/go-applereceipt"
	"github.com/devsisters/go-applereceipt/applepki"

	"github.com/code-payments/flipchat-server/iap"
)

type AppleVerifier struct{}

func NewAppleVerifier() iap.Verifier {
	return &AppleVerifier{}
}

func (m *AppleVerifier) VerifyReceipt(ctx context.Context, encodedReceipt string) (bool, error) {

	receipt, err := applereceipt.DecodeBase64(encodedReceipt, applepki.CertPool())
	if err != nil {
		// Not returning an error here because we're testing the verifier, not the
		// receipt parsing.

		return false, nil
	}

	// Verify the bundle ID.
	if receipt.BundleIdentifier != "com.flipchat.app" {
		return false, nil
	}

	// Verify the that the receipt is for the correct product.
	if receipt.InAppPurchaseReceipts[0].ProductIdentifier != "com.flipchat.iap.createAccount" {
		return false, nil
	}

	// TODO: verify the AppVersion field in the receipt?
	// receipt.AppVersion

	return true, nil
}

func (m *AppleVerifier) GetReceiptIdentifier(ctx context.Context, encodedReceipt string) ([]byte, error) {

	// TODO: adjust this so that verification and getting the identifier don't
	// require decoding the receipt twice. Once we know how to decode an Android
	// receipt we can do this.

	receipt, err := applereceipt.DecodeBase64(encodedReceipt, applepki.CertPool())
	if err != nil {
		return nil, err
	}

	return receipt.SHA1Hash, nil
}
