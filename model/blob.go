package model

import (
	"fmt"

	"github.com/mr-tron/base58"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

func MustGenerateBlobID() *commonpb.BlobId {
	id, err := GenerateBlobID()
	if err != nil {
		panic(fmt.Sprintf("failed to generate blob id: %v", err))
	}

	return id
}

func GenerateBlobID() (*commonpb.BlobId, error) {
	kp, err := GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	return &commonpb.BlobId{Value: kp.Public()}, nil
}

func BlobIDString(blob *commonpb.BlobId) string {
	return base58.Encode(blob.Value)
}
