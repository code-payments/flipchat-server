package blob

import (
	"errors"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	blobpb "github.com/code-payments/flipchat-protobuf-api/generated/go/blob/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/model"
)

// Generates a unique BlobId
func generateBlobID() *commonpb.BlobId {
	return model.MustGenerateBlobID()
}

// Converts protobuf BlobType to internal BlobType
func fromProtoBlobType(protoType blobpb.BlobType) (BlobType, error) {
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
func toProtoBlobType(internalType BlobType) blobpb.BlobType {
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
func toProtoBlob(blob *Blob) (*blobpb.Blob, error) {
	protoBlob := &blobpb.Blob{
		BlobId:    blob.ID,
		BlobType:  toProtoBlobType(blob.Type),
		OwnerId:   blob.UserID,
		S3Url:     blob.S3URL,
		CreatedAt: timestamppb.New(blob.CreatedAt),
		//Size:      blob.Size,
		//Flagged:   blob.Flagged,
	}

	// Unmarshal metadata based on BlobType
	switch blob.Type {
	case BlobTypeImage:
		var imgMeta blobpb.ImageMetadata
		if err := proto.Unmarshal(blob.Metadata, &imgMeta); err != nil {
			return nil, errors.New("failed to unmarshal image metadata")
		}
		protoBlob.Metadata = &blobpb.Blob_ImageMetadata{
			ImageMetadata: &imgMeta,
		}
	case BlobTypeVideo:
		var vidMeta blobpb.VideoMetadata
		if err := proto.Unmarshal(blob.Metadata, &vidMeta); err != nil {
			return nil, errors.New("failed to unmarshal video metadata")
		}
		protoBlob.Metadata = &blobpb.Blob_VideoMetadata{
			VideoMetadata: &vidMeta,
		}
	case BlobTypeAudio:
		var audMeta blobpb.AudioMetadata
		if err := proto.Unmarshal(blob.Metadata, &audMeta); err != nil {
			return nil, errors.New("failed to unmarshal audio metadata")
		}
		protoBlob.Metadata = &blobpb.Blob_AudioMetadata{
			AudioMetadata: &audMeta,
		}
	default:
		// BlobTypeUnknown or unhandled type
		return nil, errors.New("failed to unmarshal metadata")
	}

	return protoBlob, nil
}
