package blob

import (
	"bytes"
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	blobpb "github.com/code-payments/flipchat-protobuf-api/generated/go/blob/v1"

	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/image"
	"github.com/code-payments/flipchat-server/s3"
)

const loginWindow = 2 * time.Minute

type Server struct {
	log *zap.Logger

	blobStore Store
	s3Store   s3.Store
	authz     auth.Authorizer

	blobpb.UnimplementedBlobServiceServer
}

func NewServer(
	log *zap.Logger,
	authz auth.Authorizer,
	blobStore Store,
	s3Store s3.Store,
) *Server {
	return &Server{
		log:       log,
		authz:     authz,
		blobStore: blobStore,
		s3Store:   s3Store,
	}
}

func (s *Server) Upload(ctx context.Context, req *blobpb.UploadBlobRequest) (*blobpb.UploadBlobResponse, error) {
	// Validate the request
	if req.GetOwnerId() == nil {
		return nil, status.Error(codes.InvalidArgument, "owner_id is required")
	}
	if req.GetBlobType() == blobpb.BlobType_BLOB_TYPE_UNKNOWN {
		return nil, status.Error(codes.InvalidArgument, "blob_type is required")
	}
	if len(req.GetRawData()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "raw_data is required")
	}

	// Check if the user is authorized to upload the blob
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	// Check if the owner_id matches the authenticated user
	if bytes.Compare(userID.Value, req.GetOwnerId().Value) != 0 {
		return nil, status.Error(codes.PermissionDenied, "owner_id does not match authenticated user")
	}

	blobId := GenerateBlobID()

	// Convert BlobType from protobuf to internal type
	blobType, err := FromProtoBlobType(req.GetBlobType())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid blob_type")
	}

	// Upload raw_data to S3
	key, err := s3.ToS3Key(blobId.Value)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to generate s3 url")
	}

	s3Url, err := s.s3Store.Upload(ctx, key, req.GetRawData())
	if err != nil {
		s.log.Error("Failed to upload blob to S3", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to upload blob")
	}

	// Process the raw_data based on the BlobType
	metadata := &blobpb.Blob_Metadata{Version: 0}
	switch blobType {
	case BlobTypeImage:
		info, err := image.ProcessImage(req.GetRawData())
		if err != nil {
			s.log.Error("Failed to process image", zap.Error(err))
		} else {
			metadata.Info = &blobpb.Blob_Metadata_Image{Image: info}
		}
	case BlobTypeVideo:
		// Not implemented yet
		s.log.Warn("Video blob type not implemented")
	case BlobTypeAudio:
		// Not implemented yet
		s.log.Warn("Video blob type not implemented")
	default:
		// Not Supported
		return nil, status.Error(codes.InvalidArgument, "blob_type not supported")
	}

	serializedMetadata, err := proto.Marshal(metadata)
	if err != nil {
		s.log.Error("Failed to marshal metadata", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to marshal metadata")
	}

	// Create the Blob record
	blob := &Blob{
		ID:        blobId,
		UserID:    req.GetOwnerId(),
		Type:      blobType,
		S3URL:     s3Url,
		Size:      int64(len(req.GetRawData())),
		Metadata:  serializedMetadata,
		Flagged:   false,
		CreatedAt: time.Now(),
	}

	if err := s.blobStore.CreateBlob(ctx, blob); err != nil {
		if errors.Is(err, ErrExists) {
			return nil, status.Error(codes.AlreadyExists, "blob already exists")
		}
		s.log.Error("Failed to store blob metadata", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to store blob metadata")
	}

	// Prepare response
	responseBlob, err := ToProtoBlob(blob)
	if err != nil {
		s.log.Error("Failed to convert Blob to proto", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to prepare response")
	}

	return &blobpb.UploadBlobResponse{
		Blob: responseBlob,
	}, nil
}

func (s *Server) GetInfo(ctx context.Context, req *blobpb.GetBlobInfoRequest) (*blobpb.GetBlobInfoResponse, error) {
	// Validate the request
	if req.GetBlobId() == nil {
		return nil, status.Error(codes.InvalidArgument, "blob_id is required")
	}

	// Retrieve Blob from the store
	blob, err := s.blobStore.GetBlob(ctx, req.GetBlobId())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "blob not found")
		}
		s.log.Error("Failed to get blob from store", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to retrieve blob: %v", err)
	}

	// Convert Blob to blobpb.Blob
	blobPB, err := ToProtoBlob(blob)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal blob: %v", err)
	}

	return &blobpb.GetBlobInfoResponse{
		Blob: blobPB,
	}, nil
}
