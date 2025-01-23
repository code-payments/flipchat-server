package s3

import (
	"context"
	"errors"
)

const (
	// Cannot be changed using 0.17.0 aws-sdk-go-v2
	S3_Region = "us-east-1"

	// S3_BaseURL is the base URL for S3 objects.
	S3_BaseURL = "https://%s.s3.%s.amazonaws.com/%s"
)

var ErrNotFound = errors.New("key not found")

// Defines the interface for managing data in S3.
type Store interface {
	// Uploads the data to S3 at the specified key and returns the URL path to
	// the uploaded data.
	Upload(ctx context.Context, key string, data []byte) (string, error)

	// Download the data at a given path from S3
	Download(ctx context.Context, key string) ([]byte, error)
}
