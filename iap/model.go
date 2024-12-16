package iap

import (
	"crypto/sha256"
	"encoding/hex"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	iappb "github.com/code-payments/flipchat-protobuf-api/generated/go/iap/v1"
)

// todo: pull in Z's branch
func GetReceiptId(receipt *iappb.Receipt, platform commonpb.Platform) string {
	hasher := sha256.New()
	hasher.Write([]byte(receipt.Value))
	return hex.EncodeToString(hasher.Sum(nil))
}
