package postgres

import (
	"context"
	"errors"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	"github.com/code-payments/flipchat-server/database/prisma/db"
	"github.com/code-payments/flipchat-server/preview"
)

type store struct {
	client *db.PrismaClient
}

// reset clears the preview table (used for testing).
func (s *store) reset() {
	ctx := context.Background()

	previews := s.client.Preview.FindMany().Delete().Tx()
	err := s.client.Prisma.Transaction(previews).Exec(ctx)
	if err != nil {
		panic(err)
	}
}

// NewInPostgres creates a new PostgreSQL store for Previews.
func NewInPostgres(client *db.PrismaClient) preview.Store {
	return &store{
		client: client,
	}
}

// GetPreviewByID retrieves a preview by ID from PostgreSQL.
func (s *store) GetPreviewByID(ctx context.Context, id string) (*preview.Preview, error) {
	res, err := s.client.Preview.FindUnique(
		db.Preview.ID.Equals(id),
	).Exec(ctx)

	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}

	if res == nil {
		return nil, preview.ErrNotFound
	}

	return mapPrismaPreviewToPreview(res), nil
}

// GetPreviewByOriginalURL retrieves a preview by the original URL from PostgreSQL.
func (s *store) GetPreviewByOriginalURL(ctx context.Context, originalURL string) (*preview.Preview, error) {
	res, err := s.client.Preview.FindUnique(
		db.Preview.OriginalURL.Equals(originalURL),
	).Exec(ctx)

	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}

	if res == nil {
		return nil, preview.ErrNotFound
	}

	return mapPrismaPreviewToPreview(res), nil
}

// CreatePreview creates a new preview in PostgreSQL.
func (s *store) CreatePreview(ctx context.Context, p *preview.Preview) error {
	_, err := s.client.Preview.CreateOne(
		db.Preview.ID.Set(p.ID),
		db.Preview.OriginalURL.Set(p.OriginalURL),
		db.Preview.URL.Set(p.URL),
		db.Preview.Title.Set(p.Title),
		db.Preview.Description.Set(p.Description),
		db.Preview.ImageURL.Set(p.ImageURL),
		db.Preview.ImageHash.Set(p.ImageHash),
		db.Preview.ImageWidth.Set(p.ImageWidth),
		db.Preview.ImageHeight.Set(p.ImageHeight),
		db.Preview.ContentType.Set(int(p.ContentType)),
		db.Preview.Moderation.Set(int(p.Moderation)),
	).Exec(ctx)
	if err != nil {
		return preview.ErrExists
	}

	return nil
}

// UpdatePreview updates an existing preview in PostgreSQL.
func (s *store) UpdatePreview(ctx context.Context, p *preview.Preview) error {
	_, err := s.client.Preview.FindUnique(
		db.Preview.ID.Equals(p.ID),
	).Update(
		db.Preview.URL.Set(p.URL),
		db.Preview.Title.Set(p.Title),
		db.Preview.Description.Set(p.Description),
		db.Preview.ImageURL.Set(p.ImageURL),
		db.Preview.ImageHash.Set(p.ImageHash),
		db.Preview.ImageWidth.Set(p.ImageWidth),
		db.Preview.ImageHeight.Set(p.ImageHeight),
		db.Preview.ContentType.Set(int(p.ContentType)),
		db.Preview.Moderation.Set(int(p.Moderation)),
	).Exec(ctx)
	if err != nil {
		return preview.ErrNotFound
	}
	return nil
}

// DeletePreview deletes a preview by ID from PostgreSQL.
func (s *store) DeletePreview(ctx context.Context, id string) error {
	_, err := s.client.Preview.FindUnique(
		db.Preview.ID.Equals(id),
	).Delete().Exec(ctx)
	if err != nil {
		return preview.ErrNotFound
	}
	return nil
}

// mapPrismaPreviewToPreview maps Prisma Preview to Preview struct.
func mapPrismaPreviewToPreview(model *db.PreviewModel) *preview.Preview {
	return &preview.Preview{
		ID:          model.ID,
		OriginalURL: model.OriginalURL,
		ContentType: commonpb.ContentType(model.ContentType),
		Moderation:  commonpb.ModerationStatus(model.Moderation),
		URL:         model.URL,
		Title:       model.Title,
		Description: model.Description,
		ImageURL:    model.ImageURL,
		ImageHash:   model.ImageHash,
		ImageWidth:  model.ImageWidth,
		ImageHeight: model.ImageHeight,
		CreatedAt:   model.CreatedAt,
		UpdatedAt:   model.UpdatedAt,
	}
}
