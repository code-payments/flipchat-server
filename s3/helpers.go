package s3

import (
	"encoding/hex"
	"fmt"
	"net/url"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

// TODO: load these from environment variables or a config file.
const (
	S3Region       = "us-east-1"
	S3Bucket       = "bucket-name"
	S3BaseURL      = "https://%s.s3.%s.amazonaws.com/"
	BlobPathPrefix = "blobs/"
)

// Generates a full S3 URL for a given blobId.
// It encodes the blobId as a hexadecimal string to ensure it's URL-safe.
func GenerateS3URLPathForBlob(blobId *commonpb.BlobId) (string, error) {
	if blobId == nil {
		return "", fmt.Errorf("blobId cannot be nil")
	}

	// Encode blobId to a hex string
	encodedBlobId := hex.EncodeToString(blobId.Value)
	objectKey := fmt.Sprintf("%s%s", BlobPathPrefix, encodedBlobId)
	encodedObjectKey := url.PathEscape(objectKey)

	// Construct the full S3 URL
	s3URL := fmt.Sprintf(S3BaseURL, S3Bucket, S3Region) + encodedObjectKey

	return s3URL, nil
}
