package blob

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	blobpb "github.com/code-payments/flipchat-protobuf-api/generated/go/blob/v1"

	"github.com/code-payments/flipchat-server/s3"
)

const loginWindow = 2 * time.Minute

type Server struct {
	log     *zap.Logger
	store   Store
	s3Store s3.Store

	blobpb.UnimplementedBlobServiceServer
}

func NewServer(log *zap.Logger, store Store, s3Store s3.Store) *Server {
	return &Server{
		log:     log,
		store:   store,
		s3Store: s3Store,
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

	blobId := generateBlobID()

	// Convert BlobType from protobuf to internal type
	blobType, err := fromProtoBlobType(req.GetBlobType())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid blob_type")
	}

	// Upload raw_data to S3
	s3Url, err := s3.GenerateS3URLPathForBlob(blobId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to generate s3 url")
	}

	err = s.s3Store.Upload(ctx, s3Url, req.GetRawData())
	if err != nil {
		s.log.Error("Failed to upload blob to S3", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to upload blob")
	}

	// Create the Blob record
	blob := &Blob{
		ID:        blobId,
		UserID:    req.GetOwnerId(),
		Type:      blobType,
		S3URL:     s3Url,
		Size:      int64(len(req.GetRawData())),
		Metadata:  []byte("version_0: empty"),
		Flagged:   false,
		CreatedAt: time.Now(),
	}

	if err := s.store.CreateBlob(ctx, blob); err != nil {
		if errors.Is(err, ErrExists) {
			return nil, status.Error(codes.AlreadyExists, "blob already exists")
		}
		s.log.Error("Failed to store blob metadata", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to store blob metadata")
	}

	// Prepare response
	responseBlob, err := toProtoBlob(blob)
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
	blob, err := s.store.GetBlob(ctx, req.GetBlobId())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "blob not found")
		}
		s.log.Error("Failed to get blob from store", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to retrieve blob: %v", err)
	}

	// Convert Blob to blobpb.Blob
	blobPB, err := toProtoBlob(blob)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal blob: %v", err)
	}

	return &blobpb.GetBlobInfoResponse{
		Blob: blobPB,
	}, nil
}
