package postgres

import (
	"context"
	"errors"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	pg "github.com/code-payments/flipchat-server/database/postgres"

	"github.com/code-payments/flipchat-server/database/prisma/db"
	"github.com/code-payments/flipchat-server/intent"
)

type store struct {
	client *db.PrismaClient
}

func NewInPostgres(client *db.PrismaClient) intent.Store {
	return &store{
		client,
	}
}

func (s *store) reset() {
	ctx := context.Background()

	intents := s.client.Intent.FindMany().Delete().Tx()

	err := s.client.Prisma.Transaction(intents).Exec(ctx)
	if err != nil {
		panic(err)
	}
}

func (s *store) IsFulfilled(ctx context.Context, id *commonpb.IntentId) (bool, error) {
	encodedIntentID := pg.Encode(id.Value, pg.Base58)

	intent, err := s.client.Intent.FindFirst(
		db.Intent.ID.Equals(encodedIntentID),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) || intent == nil {
		return false, nil
	}

	return intent.IsFulfilled, nil
}

func (s *store) MarkFulfilled(ctx context.Context, id *commonpb.IntentId) error {
	encodedIntentID := pg.Encode(id.Value, pg.Base58)

	ok, err := s.IsFulfilled(ctx, id)
	if err != nil {
		return err
	}

	if ok {
		return intent.ErrAlreadyFulfilled
	}

	// Upsert the intent with the new fulfilled status
	_, err = s.client.Intent.UpsertOne(
		db.Intent.ID.Equals(encodedIntentID),
	).Create(
		db.Intent.ID.Set(encodedIntentID),
		db.Intent.IsFulfilled.Set(true),
	).Update(
		db.Intent.IsFulfilled.Set(true),
	).Exec(ctx)

	return err
}
