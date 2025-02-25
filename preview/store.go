package preview

import (
	"context"
	"errors"
	"time"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

var (
	ErrNotFound = errors.New("preview not found")
	ErrExists   = errors.New("preview already exists")
)

// Store defines the interface for Preview storage.
type Store interface {
	// GetPreviewByID retrieves a preview by ID.
	GetPreviewByID(ctx context.Context, id *commonpb.PreviewId) (*Preview, error)

	// GetPreviewByOriginalURL retrieves a preview by the original URL.
	GetPreviewByOriginalURL(ctx context.Context, originalURL string) (*Preview, error)

	// CreatePreview creates a new preview.
	CreatePreview(ctx context.Context, preview *Preview) error

	// UpdatePreview updates an existing preview.
	UpdatePreview(ctx context.Context, preview *Preview) error

	// DeletePreview deletes a preview by ID.
	DeletePreview(ctx context.Context, id *commonpb.PreviewId) error
}

// Preview represents a preview.
type Preview struct {
	ID *commonpb.PreviewId

	OriginalURL string
	ContentType commonpb.ContentType
	Moderation  commonpb.ModerationStatus

	URL         string
	Title       string
	Description string

	ImageURL    string
	ImageHash   string
	ImageWidth  int
	ImageHeight int

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Creates a copy of the preview.
func (p *Preview) Clone() *Preview {
	return &Preview{
		ID: &commonpb.PreviewId{Value: p.ID.Value},

		OriginalURL: p.OriginalURL,
		ContentType: p.ContentType,
		Moderation:  p.Moderation,

		URL:         p.URL,
		Title:       p.Title,
		Description: p.Description,

		ImageURL:    p.ImageURL,
		ImageHash:   p.ImageHash,
		ImageWidth:  p.ImageWidth,
		ImageHeight: p.ImageHeight,

		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}
