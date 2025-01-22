package s3

import (
	"context"
)

// Defines the interface for managing data in S3.
type Store interface {
	// Uploads the data to S3
	Upload(ctx context.Context, key string, data []byte) error

	// Download the data at a given path from S3
	Download(ctx context.Context, key string) ([]byte, error)
}
