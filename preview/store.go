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
	GetPreviewByID(ctx context.Context, id string) (*Preview, error)

	// GetPreviewByOriginalURL retrieves a preview by the original URL.
	GetPreviewByOriginalURL(ctx context.Context, originalURL string) (*Preview, error)

	// CreatePreview creates a new preview.
	CreatePreview(ctx context.Context, preview *Preview) error

	// UpdatePreview updates an existing preview.
	UpdatePreview(ctx context.Context, preview *Preview) error

	// DeletePreview deletes a preview by ID.
	DeletePreview(ctx context.Context, id string) error
}

// Preview represents a preview.
type Preview struct {
	ID string

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
