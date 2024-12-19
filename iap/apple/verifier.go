package apple

import (
	"context"

	"github.com/devsisters/go-applereceipt"
	"github.com/devsisters/go-applereceipt/applepki"

	"github.com/code-payments/flipchat-server/iap"
)

type AppleVerifier struct {
	// PackageName is the app's package name, e.g. "com.flipchat.app".
	packageName string

	// ProductName is the name of the product that the receipt should be for.
	productName string
}

func NewAppleVerifier(pkgName string, product string) iap.Verifier {
	return &AppleVerifier{
		packageName: pkgName,
		productName: product,
	}
}

func (m *AppleVerifier) VerifyReceipt(ctx context.Context, encodedReceipt string) (bool, error) {

	receipt, err := applereceipt.DecodeBase64(encodedReceipt, applepki.CertPool())
	if err != nil {
		// Not returning an error here because we're testing the verifier, not the
		// receipt parsing.

		return false, nil
	}

	// Verify the bundle ID.
	if receipt.BundleIdentifier != m.packageName {
		return false, nil
	}

	// Verify the that the receipt is for the correct product.
	if receipt.InAppPurchaseReceipts[0].ProductIdentifier != m.productName {
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
