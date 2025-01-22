package s3

import (
	"context"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

// Defines the interface for uploading and managing blob data in S3.
type Store interface {

	// Uploads the blob data to S3 and returns the S3 URL.
	UploadBlob(ctx context.Context, ownerID *commonpb.UserId, blobID *commonpb.BlobId, data []byte) (string, error)

	// Uploads the blob data to S3 using a stream and returns the S3 URL.
	UploadBlobStream(ctx context.Context, ownerID *commonpb.UserId, blobID *commonpb.BlobId, reader io.Reader) (string, error)
}
