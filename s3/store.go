package s3

import (
	"context"
)

// Defines the interface for uploading and managing blob data in S3.
type Store interface {
	// Uploads the data to S3 and returns the S3 URL.
	Upload(ctx context.Context, key string, data []byte) error
}
