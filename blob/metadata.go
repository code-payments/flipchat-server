package blob

import (
	"errors"

	"google.golang.org/protobuf/proto"

	blobpb "github.com/code-payments/flipchat-protobuf-api/generated/go/blob/v1"
)

func newMetadata(blobType BlobType) ([]byte, error) {
	var metadataBytes []byte
	var err error

	switch blobType {
	case BlobTypeImage:
		imgMeta := &blobpb.ImageMetadata{
			Version: 0,

			/*
				// Example metadata, replace with actual data
				Width:   800,
				Height:  600,
				Format:  blobpb.ImageMetadata_IMAGE_FORMAT_PNG,
			*/
		}
		metadataBytes, err = proto.Marshal(imgMeta)
		if err != nil {
			return nil, errors.New("failed to marshal image metadata: " + err.Error())
		}
	case BlobTypeVideo:
		vidMeta := &blobpb.VideoMetadata{
			Version: 0,

			/*
				// Example metadata, replace with actual data
				Width:           1920,
				Height:          1080,
				DurationSeconds: 120,
				FrameRate:       30.0,
				Codec:           "H.264",
			*/
		}
		metadataBytes, err = proto.Marshal(vidMeta)
		if err != nil {
			return nil, errors.New("failed to marshal video metadata: " + err.Error())
		}
	case BlobTypeAudio:
		audMeta := &blobpb.AudioMetadata{
			Version: 0,

			/*
				// Example metadata, replace with actual data
				DurationSeconds: 180,
				Codec:           "AAC",
			*/
		}
		metadataBytes, err = proto.Marshal(audMeta)
		if err != nil {
			return nil, errors.New("failed to marshal audio metadata: " + err.Error())
		}
	default:
		// BlobTypeUnknown
		return nil, errors.New("failed to process metadata: unknown blob type")
	}

	return metadataBytes, nil
}
