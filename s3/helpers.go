package s3

import (
	"encoding/hex"
	"fmt"
)

// Encodes the byte id as a hexadecimal string to ensure it's URL-safe.
func ToS3Key(byteId []byte) (string, error) {
	if byteId == nil {
		return "", fmt.Errorf("byteId cannot be nil")
	}

	return hex.EncodeToString(byteId), nil
}

// Returns the S3 URL path for the given key, bucket, and region.
func GenerateS3URLPathForByteId(key string, bucket string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("key cannot be empty")
	}
	if bucket == "" {
		return "", fmt.Errorf("bucket cannot be empty")
	}

	url := fmt.Sprintf(S3_BaseURL, bucket, S3_Region, key)

	return url, nil
}
