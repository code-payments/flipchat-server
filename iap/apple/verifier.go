package apple

import (
	"context"
	"crypto/x509"
	_ "embed"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strconv"
	"time"

	"github.com/code-payments/flipchat-server/iap"
	"github.com/fullsailor/pkcs7"
)

// AppleReceipt describes the essential receipt fields Apple documents.
type AppleReceipt struct {
	// Bundle Identifier (Type 2)
	BundleID string

	// App Version (Type 3)
	ApplicationVersion string

	// Opaque Value (Type 4)
	OpaqueValue []byte

	// Receipt Hash (Type 5) - SKIPPED for device hash validation
	// Left in the structure for completeness, but we won't require a match.
	ReceiptHash []byte

	// In-App Purchase receipts (Type 17)
	InApp []InAppReceipt

	// Original Application Version (Type 19)
	OriginalApplicationVersion string

	// Receipt Creation Date (Type 12)
	ReceiptCreationDate time.Time

	// Receipt Expiration Date (Type 21)
	ReceiptExpirationDate *time.Time
}

// InAppReceipt describes a single in-app purchase receipt (Type 17).
type InAppReceipt struct {
	// Quantity (Type 1701)
	Quantity int

	// Product Identifier (Type 1702)
	ProductID string

	// Transaction Identifier (Type 1703)
	TransactionID string

	// Purchase Date (Type 1704)
	PurchaseDate time.Time

	// Original Transaction Identifier (Type 1705)
	OriginalTransactionID string

	// Original Purchase Date (Type 1706)
	OriginalPurchaseDate time.Time

	// Subscription Expiration Date (Type 1708)
	SubscriptionExpirationDate *time.Time

	// Web Order Line Item ID (Type 1711)
	WebOrderLineItemID *int64

	// Cancellation Date (Type 1712)
	CancellationDate *time.Time

	// Is in Intro Offer Period (Type 1719)
	IsInIntroOfferPeriod *bool
}

// AppleVerifier verifies local Apple receipts using PKCS#7 parsing and manual chain checks.
// We do NOT validate the device hash here, per your request.
type AppleVerifier struct {
	// AppleRootCAPem is the PEM-encoded Apple Root CA
	AppleRootCAPem []byte

	// ExpectedBundleID and ExpectedVersion are optional checks
	ExpectedBundleID string
	ExpectedVersion  string
}

// NewAppleVerifier creates a new AppleVerifier.
func NewAppleVerifier(rootCAPem []byte, bundleID, version string) iap.Verifier {
	return &AppleVerifier{
		AppleRootCAPem:   rootCAPem,
		ExpectedBundleID: bundleID,
		ExpectedVersion:  version,
	}
}

// VerifyReceipt decodes the base64 PKCS7, verifies the signature digest (not chain),
// then manually verifies the chain to Apple Root, and parses the receipt fields.
func (v *AppleVerifier) VerifyReceipt(ctx context.Context, receipt string) (bool, error) {
	raw, err := base64.StdEncoding.DecodeString(receipt)
	if err != nil {
		return false, nil
	}

	p7, err := pkcs7.Parse(raw)
	if err != nil {
		return false, nil
	}
	// Verify PKCS#7 integrity (signer digest matches content).
	if err := p7.Verify(); err != nil {
		return false, nil
	}
	// Manually verify the certificate chain back to Apple’s root.
	if !v.verifyChain(p7.Certificates) {
		return false, nil
	}

	// Parse the top-level ASN.1 SET OF receiptAttribute
	attrs, err := parseASN1ReceiptAttributes(p7.Content)
	if err != nil {
		return false, nil
	}

	appleReceipt := &AppleReceipt{}
	if err := parseAppleReceiptAttributes(attrs, appleReceipt); err != nil {
		return false, nil
	}

	// Optional checks for bundle ID and version
	if v.ExpectedBundleID != "" && appleReceipt.BundleID != v.ExpectedBundleID {
		return false, nil
	}
	if v.ExpectedVersion != "" && appleReceipt.ApplicationVersion != v.ExpectedVersion {
		return false, nil
	}

	// Skipping device hash validation.
	// Each installation of an app has a unique Device/UUID hash and our
	// database will prevent the same receipt being stored multiple times.
	// All we care about is that the receipt is valid and "a" user has
	// paid for the feature. We don't need to validate that "this" user
	// has paid for the feature in this context.

	return true, nil
}

// verifyChain checks that the first certificate in p7.Certificates chains to Apple’s Root CA.
func (v *AppleVerifier) verifyChain(certs []*x509.Certificate) bool {
	if len(certs) == 0 {
		return false
	}
	rootCA, err := loadCertificateFromPEM(v.AppleRootCAPem)
	if err != nil {
		return false
	}
	rootPool := x509.NewCertPool()
	rootPool.AddCert(rootCA)

	leaf := certs[0]
	intermediates := x509.NewCertPool()
	for _, icert := range certs[1:] {
		intermediates.AddCert(icert)
	}

	opts := x509.VerifyOptions{
		Roots:         rootPool,
		Intermediates: intermediates,
		// Production code might set CurrentTime to appleReceipt.ReceiptCreationDate
	}
	if _, err := leaf.Verify(opts); err != nil {
		return false
	}
	return true
}

// parseAppleReceiptAttributes extracts top-level receipt fields, including in-app purchases (type 17).
func parseAppleReceiptAttributes(attrs []receiptAttribute, r *AppleReceipt) error {
	for _, attr := range attrs {
		switch attr.Type {
		case 2: // Bundle Identifier
			r.BundleID = string(attr.Value)
		case 3: // App Version
			r.ApplicationVersion = string(attr.Value)
		case 4: // Opaque Value
			r.OpaqueValue = attr.Value
		case 5:
			// Receipt Hash. Not used in this scenario, but we still store it.
			r.ReceiptHash = attr.Value
		case 17:
			inAppReceipts, err := parseInAppPurchaseAttributes(attr.Value)
			if err != nil {
				return err
			}
			r.InApp = append(r.InApp, inAppReceipts...)
		case 19:
			r.OriginalApplicationVersion = string(attr.Value)
		case 12: // Receipt Creation Date (RFC3339)
			t, err := parseRFC3339Time(attr.Value)
			if err != nil {
				return err
			}
			r.ReceiptCreationDate = t
		case 21: // Receipt Expiration Date (RFC3339)
			t, err := parseRFC3339Time(attr.Value)
			if err != nil {
				return err
			}
			r.ReceiptExpirationDate = &t
		default:
			// Ignore unrecognized or reserved fields
		}
	}
	return nil
}

// parseInAppPurchaseAttributes decodes the ASN.1 SET OF receiptAttribute for a single in-app purchase (type 17).
func parseInAppPurchaseAttributes(raw []byte) ([]InAppReceipt, error) {
	var set asn1.RawValue
	rest, err := asn1.Unmarshal(raw, &set)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("extra data after in-app purchase set parsing")
	}
	if set.Tag != asn1.TagSet {
		return nil, fmt.Errorf("expected SET (asn1.TagSet) for in-app purchase, got tag %d", set.Tag)
	}

	inAppAttrs := set.Bytes
	var iapAttrs []receiptAttribute
	for len(inAppAttrs) > 0 {
		var attr receiptAttribute
		next, err := asn1.Unmarshal(inAppAttrs, &attr)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal in-app attribute: %w", err)
		}
		iapAttrs = append(iapAttrs, attr)
		inAppAttrs = next
	}

	iap := InAppReceipt{}
	if err := parseSingleInAppReceipt(iapAttrs, &iap); err != nil {
		return nil, err
	}
	return []InAppReceipt{iap}, nil
}

// parseSingleInAppReceipt populates an InAppReceipt from a slice of receiptAttribute fields.
func parseSingleInAppReceipt(attrs []receiptAttribute, iap *InAppReceipt) error {
	for _, attr := range attrs {
		switch attr.Type {
		case 1701: // Quantity
			q, err := parseASN1Integer(attr.Value)
			if err != nil {
				return err
			}
			iap.Quantity = int(q)
		case 1702: // Product Identifier
			iap.ProductID = string(attr.Value)
		case 1703: // Transaction Identifier
			iap.TransactionID = string(attr.Value)
		case 1704: // Purchase Date
			t, err := parseRFC3339Time(attr.Value)
			if err != nil {
				return err
			}
			iap.PurchaseDate = t
		case 1705: // Original Transaction ID
			iap.OriginalTransactionID = string(attr.Value)
		case 1706: // Original Purchase Date
			t, err := parseRFC3339Time(attr.Value)
			if err != nil {
				return err
			}
			iap.OriginalPurchaseDate = t
		case 1708: // Subscription Expiration Date
			t, err := parseRFC3339Time(attr.Value)
			if err == nil {
				iap.SubscriptionExpirationDate = &t
			}
		case 1711: // Web Order Line Item ID
			wid, err := parseASN1Integer(attr.Value)
			if err == nil {
				iap.WebOrderLineItemID = &wid
			}
		case 1712: // Cancellation Date
			t, err := parseRFC3339Time(attr.Value)
			if err == nil {
				iap.CancellationDate = &t
			}
		case 1719: // Is in Intro Offer Period
			b, err := parseASN1Integer(attr.Value)
			if err == nil {
				val := (b == 1)
				iap.IsInIntroOfferPeriod = &val
			}
		default:
			// Ignore unknown IAP fields
		}
	}
	return nil
}

// parseASN1ReceiptAttributes parses a top-level ASN.1 SET OF receiptAttribute.
func parseASN1ReceiptAttributes(raw []byte) ([]receiptAttribute, error) {
	var set asn1.RawValue
	rest, err := asn1.Unmarshal(raw, &set)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("extra data after top-level receipt parse")
	}
	if set.Tag != asn1.TagSet {
		return nil, fmt.Errorf("expected SET (asn1.TagSet), got tag %d", set.Tag)
	}

	var result []receiptAttribute
	data := set.Bytes
	for len(data) > 0 {
		var attr receiptAttribute
		next, err := asn1.Unmarshal(data, &attr)
		if err != nil {
			return nil, err
		}
		result = append(result, attr)
		data = next
	}
	return result, nil
}

// receiptAttribute is Apple's ASN.1 structure: SEQUENCE { type (INTEGER), version (INTEGER), value (OCTET STRING) }
type receiptAttribute struct {
	Type    int    `asn1:"tag:0"`
	Version int    `asn1:"tag:1"`
	Value   []byte `asn1:"tag:2,octet"`
}

// parseASN1Integer tries to decode a DER-encoded INTEGER or a UTF8STRING that contains a numeric value.
func parseASN1Integer(value []byte) (int64, error) {
	var num int64
	rest, err := asn1.Unmarshal(value, &num)
	if err == nil && len(rest) == 0 {
		return num, nil
	}
	// Fallback: interpret value as decimal string
	strVal := string(value)
	num, err = strconv.ParseInt(strVal, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("unable to parse integer from: %q", strVal)
	}
	return num, nil
}

// parseRFC3339Time interprets attr.Value as an RFC3339 date string (e.g. "2023-01-01T12:34:56Z").
func parseRFC3339Time(value []byte) (time.Time, error) {
	strVal := string(value)
	t, err := time.Parse(time.RFC3339, strVal)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid RFC3339 date: %s", strVal)
	}
	return t, nil
}

func loadCertificateFromPEM(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to decode root CA PEM block")
	}
	return x509.ParseCertificate(block.Bytes)
}
