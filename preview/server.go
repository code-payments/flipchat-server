package preview

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	previewpb "github.com/code-payments/flipchat-protobuf-api/generated/go/preview/v1"

	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/moderation"
)

// Server implements the Preview service.
type Server struct {
	log        *zap.Logger
	store      Store
	moderation moderation.ModerationClient

	previewpb.UnimplementedPreviewServer
}

// NewServer creates a new Preview server.
func NewServer(
	log *zap.Logger,
	store Store,
	moderation moderation.ModerationClient,
) *Server {
	return &Server{
		log:        log,
		store:      store,
		moderation: moderation,
	}
}

// GetPreviewUrl handles the GetPreviewUrl RPC.
func (s *Server) GetPreviewUrl(ctx context.Context, req *previewpb.GetPreviewUrlRequest) (*previewpb.GetPreviewUrlResponse, error) {
	// Validate request
	if len(req.Url) < 1 || len(req.Url) > 2048 {
		return &previewpb.GetPreviewUrlResponse{
			Result: previewpb.GetPreviewUrlResponse_INVALID_REQUEST,
		}, nil
	}

	// Check if preview already exists
	preview, err := s.store.GetPreviewByOriginalURL(ctx, req.Url)
	if err != nil && err != ErrNotFound {
		s.log.Error("Failed to get preview from store", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to retrieve preview")
	}

	if preview != nil {
		return &previewpb.GetPreviewUrlResponse{
			Result:     previewpb.GetPreviewUrlResponse_OK,
			PreviewUrl: s.convertToProto(preview),
		}, nil
	}

	// Generate preview
	generatedPreview, err := s.generatePreview(ctx, req.Url)
	if err != nil {
		s.log.Error("Failed to generate preview", zap.Error(err))
		return &previewpb.GetPreviewUrlResponse{
			Result: previewpb.GetPreviewUrlResponse_EXTERNAL_ERROR,
		}, nil
	}

	// Flag content synchronously
	moderationResult, err := s.flagContent(ctx, generatedPreview)
	if err != nil {
		s.log.Error("Moderation failed", zap.Error(err))
		return &previewpb.GetPreviewUrlResponse{
			Result: previewpb.GetPreviewUrlResponse_INTERNAL_ERROR,
		}, nil
	}

	// Set moderation status
	if moderationResult.Flagged {
		generatedPreview.Moderation = commonpb.ModerationStatus_MODERATION_FLAGGED
	} else {
		generatedPreview.Moderation = commonpb.ModerationStatus_MODERATION_APPROVED
	}

	// Store the preview
	err = s.store.CreatePreview(ctx, generatedPreview)
	if err != nil {
		if err == ErrExists {
			return &previewpb.GetPreviewUrlResponse{
				Result: previewpb.GetPreviewUrlResponse_INTERNAL_ERROR,
			}, nil
		}
		s.log.Error("Failed to create preview in store", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to store preview")
	}

	return &previewpb.GetPreviewUrlResponse{
		Result:     previewpb.GetPreviewUrlResponse_OK,
		PreviewUrl: s.convertToProto(generatedPreview),
	}, nil
}

// generatePreview generates preview data for a given URL.
func (s *Server) generatePreview(ctx context.Context, url string) (*Preview, error) {
	// TODO: Implement actual URL fetching and parsing logic.
	// For demonstration, we'll use placeholder data.

	// Placeholder implementation
	now := time.Now()
	id := model.MustGeneratePreviewID()
	return &Preview{
		ID:          id,
		OriginalURL: url,
		ContentType: commonpb.ContentType_CONTENT_TYPE_UNKNOWN,
		Moderation:  commonpb.ModerationStatus_MODERATION_UNKNOWN,
		URL:         url,
		Title:       "Sample Title",
		Description: "Sample description for the provided URL.",
		ImageURL:    "https://example.com/image.png",
		ImageHash:   "LKO2?U%2Tw=w]~RBVZRi};RPxuwH",
		ImageWidth:  800,
		ImageHeight: 600,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// flagContent uses the moderation client to flag the content.
func (s *Server) flagContent(ctx context.Context, preview *Preview) (*moderation.ModerationResult, error) {
	// Moderate text fields
	text := preview.Title + " " + preview.Description
	textResult, err := s.moderation.ClassifyText(ctx, text)
	if err != nil {
		return nil, err
	}

	// Moderate image
	imageResult, err := s.moderation.ClassifyImage(ctx, preview.ImageURL)
	if err != nil {
		return nil, err
	}

	// Combine results
	flagged := false
	if textResult.Flagged || imageResult.Flagged {
		flagged = true
	}

	return &moderation.ModerationResult{
		Flagged:        flagged,
		CategoryScores: mergeCategoryScores(textResult.CategoryScores, imageResult.CategoryScores),
	}, nil
}

// mergeCategoryScores merges category scores from text and image moderation.
func mergeCategoryScores(a, b map[string]float64) map[string]float64 {
	result := make(map[string]float64)
	for k, v := range a {
		result[k] += v
	}
	for k, v := range b {
		result[k] += v
	}
	return result
}

// convertToProto converts Preview to PreviewUrl proto message.
func (s *Server) convertToProto(p *Preview) *previewpb.PreviewUrl {
	return &previewpb.PreviewUrl{
		Url:         p.URL,
		Title:       p.Title,
		Description: p.Description,
		Image: &previewpb.PreviewImage{
			ImageUrl:  p.ImageURL,
			ImageHash: p.ImageHash,
			Width:     int32(p.ImageWidth),
			Height:    int32(p.ImageHeight),
		},
		ContentType:      p.ContentType,
		ModerationStatus: p.Moderation,
		Ts:               timestamppb.New(p.CreatedAt),
	}
}
