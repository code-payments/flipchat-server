package blob

import (
	"errors"

	"google.golang.org/protobuf/types/known/timestamppb"

	blobpb "github.com/code-payments/flipchat-protobuf-api/generated/go/blob/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/model"
)

// Generates a unique BlobId
func GenerateBlobID() *commonpb.BlobId {
	return model.MustGenerateBlobID()
}

// Converts protobuf BlobType to internal BlobType
func FromProtoBlobType(protoType blobpb.BlobType) (BlobType, error) {
	switch protoType {
	case blobpb.BlobType_BLOB_TYPE_IMAGE:
		return BlobTypeImage, nil
	case blobpb.BlobType_BLOB_TYPE_VIDEO:
		return BlobTypeVideo, nil
	case blobpb.BlobType_BLOB_TYPE_AUDIO:
		return BlobTypeAudio, nil
	default:
		return BlobTypeUnknown, errors.New("unknown blob type")
	}
}

// toProtoBlobType converts internal BlobType to protobuf BlobType
func ToProtoBlobType(internalType BlobType) blobpb.BlobType {
	switch internalType {
	case BlobTypeImage:
		return blobpb.BlobType_BLOB_TYPE_IMAGE
	case BlobTypeVideo:
		return blobpb.BlobType_BLOB_TYPE_VIDEO
	case BlobTypeAudio:
		return blobpb.BlobType_BLOB_TYPE_AUDIO
	default:
		return blobpb.BlobType_BLOB_TYPE_UNKNOWN
	}
}

// Converts internal Blob to protobuf Blob, including unmarshaling metadata.
func ToProtoBlob(blob *Blob) (*blobpb.Blob, error) {
	protoBlob := &blobpb.Blob{
		BlobId:    blob.ID,
		BlobType:  ToProtoBlobType(blob.Type),
		OwnerId:   blob.UserID,
		S3Url:     blob.S3URL,
		CreatedAt: timestamppb.New(blob.CreatedAt),
	}
	return protoBlob, nil
}
