package android

import (
	"context"
	"fmt"

	"github.com/code-payments/flipchat-server/iap"

	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/option"
)

// AndroidVerifier uses the Google Play Developer API to verify purchase tokens.
type AndroidVerifier struct {

	// The contents of a service account JSON file.
	serviceAccountJSON []byte

	// PackageName is the Android app's package name.
	packageName string

	// ProductName is the name of the product that the receipt should be for.
	productName string
}

func NewAndroidVerifier(serviceAccountJSON []byte, pkgName string, product string) iap.Verifier {
	return &AndroidVerifier{
		serviceAccountJSON: serviceAccountJSON,
		packageName:        pkgName,
		productName:        product,
	}
}

func (v *AndroidVerifier) VerifyReceipt(ctx context.Context, receipt string) (bool, error) {
	svc, err := androidpublisher.NewService(ctx, option.WithCredentialsJSON(v.serviceAccountJSON))
	if err != nil {
		return false, fmt.Errorf("failed to create android publisher client: %w", err)
	}

	// If it's a subscription, call Purchases.Subscriptions.Get(...).
	// If it's a one-time product, call Purchases.Products.Get(...).

	call := svc.Purchases.Products.Get(v.packageName, v.productName, receipt)

	productPurchase, err := call.Context(ctx).Do()
	if err != nil {
		// If the API call fails (e.g., 404 purchase token not found), return false.
		return false, nil
	}

	// productPurchase contains fields like PurchaseState, ConsumptionState, AcknowledgementState, etc.
	// For example, PurchaseState == 0 means purchased, 1 means canceled, 2 means pending.
	// Check these fields to decide if it's valid.
	if productPurchase.PurchaseState != 0 {
		// 0 = purchased, anything else indicates an invalid / canceled / pending purchase.
		return false, nil
	}

	// If we get here, the purchase is likely valid.
	return true, nil
}

func (m *AndroidVerifier) GetReceiptIdentifier(ctx context.Context, receipt string) ([]byte, error) {
	// For Android, the receipt is usually a purchase token.

	return []byte(receipt), nil
}
