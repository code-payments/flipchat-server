package blob

import (
	"context"
	"errors"
	"time"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

var (
	ErrExists   = errors.New("blob already exists")
	ErrNotFound = errors.New("blob not found")
)

type BlobType int

const (
	BlobTypeUnknown BlobType = iota
	BlobTypeImage
	BlobTypeVideo
	BlobTypeAudio
)

// Blob holds blob info
type Blob struct {
	ID        *commonpb.BlobId
	UserID    *commonpb.UserId
	Type      BlobType
	S3URL     string
	Size      int64
	Metadata  []byte
	Flagged   bool
	CreatedAt time.Time
}

// Clone creates a deep copy
func (b *Blob) Clone() *Blob {
	metadataCopy := make([]byte, len(b.Metadata))
	copy(metadataCopy, b.Metadata)

	return &Blob{
		ID:        b.ID,
		UserID:    b.UserID,
		Type:      b.Type,
		S3URL:     b.S3URL,
		Size:      b.Size,
		Metadata:  metadataCopy,
		Flagged:   b.Flagged,
		CreatedAt: b.CreatedAt,
	}
}

// Store is an interface for blob operations
type Store interface {
	CreateBlob(ctx context.Context, blob *Blob) error
	GetBlob(ctx context.Context, id *commonpb.BlobId) (*Blob, error)
}
