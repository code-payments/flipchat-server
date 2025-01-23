package postgres

import (
	"context"
	"fmt"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	pg "github.com/code-payments/flipchat-server/database/postgres"

	"github.com/code-payments/flipchat-server/blob"
	"github.com/code-payments/flipchat-server/database/prisma/db"
)

type store struct {
	client *db.PrismaClient
}

func NewInPostgres(client *db.PrismaClient) blob.Store {
	return &store{
		client,
	}
}

func (s *store) reset() {
	ctx := context.Background()

	blobs := s.client.Blob.FindMany().Delete().Tx()
	err := s.client.Prisma.Transaction(blobs).Exec(ctx)
	if err != nil {
		panic(err)
	}
}

func (s *store) CreateBlob(ctx context.Context, b *blob.Blob) error {
	found, err := s.client.Blob.FindUnique(
		db.Blob.ID.Equals(b.ID.Value),
	).Exec(ctx)

	if err == nil && found != nil {
		return blob.ErrExists
	}

	encodedUserId := pg.Encode(b.UserID.Value)

	_, err = s.client.Blob.CreateOne(
		db.Blob.ID.Set(b.ID.Value),
		db.Blob.UserID.Set(encodedUserId),
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

func (s *store) GetBlob(ctx context.Context, id *commonpb.BlobId) (*blob.Blob, error) {
	row, err := s.client.Blob.FindUnique(
		db.Blob.ID.Equals(id.Value),
	).Exec(ctx)

	if err != nil || row == nil {
		return nil, blob.ErrNotFound
	}

	userID, err := pg.Decode(row.UserID)
	if err != nil {
		return nil, err
	}

	return &blob.Blob{
		ID:        id,
		UserID:    &commonpb.UserId{Value: userID},
		Type:      blob.BlobType(row.Type),
		S3URL:     row.S3URL,
		Size:      int64(row.Size),
		Metadata:  row.Metadata,
		Flagged:   row.Flagged,
		CreatedAt: row.CreatedAt,
	}, nil
}
