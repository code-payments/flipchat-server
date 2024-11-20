package postgres

import (
	"context"
	"errors"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	profilepb "github.com/code-payments/flipchat-protobuf-api/generated/go/profile/v1"
	pg "github.com/code-payments/flipchat-server/database/postgres"

	"github.com/code-payments/flipchat-server/database/prisma/db"
	"github.com/code-payments/flipchat-server/profile"
)

type store struct {
	client *db.PrismaClient
}

func NewPostgres(client *db.PrismaClient) profile.Store {
	return &store{
		client,
	}
}

func (s *store) reset() {
	ctx := context.Background()

	users := s.client.User.FindMany().Delete().Tx()

	err := s.client.Prisma.Transaction(users).Exec(ctx)
	if err != nil {
		panic(err)
	}
}

func (s *store) GetProfile(ctx context.Context, id *commonpb.UserId) (*profilepb.UserProfile, error) {
	encodedUserID := pg.Encode(id.Value)

	val, err := s.client.User.FindFirst(
		db.User.ID.Equals(encodedUserID),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) || val == nil {
		return nil, profile.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	// User found but the optional display name is not set
	name, ok := val.DisplayName()
	if !ok {
		return nil, profile.ErrNotFound
	}

	return &profilepb.UserProfile{
		DisplayName: name,
	}, nil
}

func (s *store) SetDisplayName(ctx context.Context, id *commonpb.UserId, displayName string) error {
	encodedUserID := pg.Encode(id.Value)

	// TODO: The upsert feels a bit weird here but the memory store does it too

	// Upsert the user with the new display name
	_, err := s.client.User.UpsertOne(
		db.User.ID.Equals(encodedUserID),
	).Create(
		db.User.ID.Set(encodedUserID),
		db.User.DisplayName.Set(displayName),
	).Update(
		db.User.DisplayName.Set(displayName),
	).Exec(ctx)

	return err
}
