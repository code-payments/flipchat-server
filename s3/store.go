package s3

import (
	"context"
	"errors"
)

// TODO: load these from environment variables or a config file.
const (
	S3Region       = "us-east-1"
	S3Bucket       = "bucket-name"
	S3BaseURL      = "https://%s.s3.%s.amazonaws.com/"
	BlobPathPrefix = "blobs/"
)

var ErrNotFound = errors.New("key not found")

// Defines the interface for managing data in S3.
type Store interface {
	// Uploads the data to S3
	Upload(ctx context.Context, key string, data []byte) error

	// Download the data at a given path from S3
	Download(ctx context.Context, key string) ([]byte, error)
}
