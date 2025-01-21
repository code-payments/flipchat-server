package postgres

/*package postgres

import (
	"context"
	"fmt"

	"github.com/code-payments/flipchat-server/blob"
	"github.com/code-payments/flipchat-server/database/prisma/db"
)

type store struct {
	client *db.PrismaClient
}

func NewPostgresStore(client *db.PrismaClient) blob.Store {
	return &store{
		client: client,
	}
}

func (s *store) CreateBlob(ctx context.Context, b *blob.Blob) error {
	found, err := s.client.Blob.FindUnique(
		db.Blob.ID.Equals(b.ID),
	).Exec(ctx)
	if err == nil && found != nil {
		return blob.ErrExists
	}
	_, err = s.client.Blob.CreateOne(
		db.Blob.ID.Set(b.ID),
		db.Blob.UserID.Set(b.UserID),
		db.Blob.Type.Set(int(b.Type)),
		db.Blob.S3URL.Set(b.S3URL),
		db.Blob.Size.Set(int(b.Size)),
		db.Blob.Metadata.Set(b.Metadata),
		db.Blob.Flagged.Set(b.Flagged),
		db.Blob.CreatedAt.Set(b.CreatedAt),
	).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create blob: %w", err)
	}
	return nil
}

func (s *store) GetBlob(ctx context.Context, id []byte) (*blob.Blob, error) {
	row, err := s.client.Blob.FindUnique(
		db.Blob.ID.Equals(id),
	).Exec(ctx)
	if err != nil || row == nil {
		return nil, blob.ErrNotFound
	}
	return &blob.Blob{
		ID:        row.ID,
		Owner:     row.UserID,
		Type:      blob.BlobType(row.Type),
		S3URL:     row.S3URL,
		Size:      int64(row.Size),
		Metadata:  row.Metadata,
		Flagged:   row.Flagged,
		CreatedAt: row.CreatedAt,
	}, nil
}
*/
