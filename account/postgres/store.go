package postgres

import (
	"context"
	"errors"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	pg "github.com/code-payments/flipchat-server/database/postgres"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/database/prisma/db"
)

type store struct {
	client *db.PrismaClient
}

func NewPostgres(client *db.PrismaClient) account.Store {
	return &store{
		client,
	}
}

func (s *store) reset() {
	s.client.PublicKey.FindMany().Delete().Exec(context.Background())
	s.client.User.FindMany().Delete().Exec(context.Background())
}

func (s *store) Bind(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {

	// Check if this pubkey is already bound to a user
	key, err := s.client.PublicKey.FindUnique(
		db.PublicKey.Key.Equals(pg.Encode(pubKey.Value)),
	).Exec(ctx)

	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}

	if key != nil {
		val, err := pg.Decode(key.UserID)
		if err != nil {
			return nil, err
		}

		// Cannot rebind without revoking first
		return &commonpb.UserId{Value: val}, nil
	}

	// Create a new user if it doesn't exist already
	userTx := s.client.User.UpsertOne(
		db.User.ID.Equals(pg.Encode(userID.Value)),
	).Create(
		db.User.ID.Set(pg.Encode(userID.Value)),
	).Update().Tx()

	// Create a new public key if it doesn't exist
	keyTx := s.client.PublicKey.UpsertOne(
		db.PublicKey.Key.Equals(pg.Encode(pubKey.Value)),
	).Create(
		db.PublicKey.Key.Set(pg.Encode(pubKey.Value)),
		db.PublicKey.User.Link(
			db.User.ID.Equals(pg.Encode(userID.Value)),
		),
	).Update(
		db.PublicKey.User.Link(
			db.User.ID.Equals(pg.Encode(userID.Value)),
		),
	).Tx()

	err = s.client.Prisma.Transaction(
		userTx,
		keyTx,
	).Exec(ctx)

	if err != nil {
		return nil, err
	}

	return &commonpb.UserId{Value: userID.Value}, nil
}

func (s *store) GetUserId(ctx context.Context, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {

	key, err := s.client.PublicKey.FindFirst(
		db.PublicKey.Key.Equals(pg.Encode(pubKey.Value)),
	).Exec(ctx)

	if err != nil || key == nil {
		return nil, account.ErrNotFound
	}

	val, err := pg.Decode(key.UserID)
	if err != nil {
		return nil, err
	}

	return &commonpb.UserId{Value: val}, nil
}

func (s *store) GetPubKeys(ctx context.Context, userID *commonpb.UserId) ([]*commonpb.PublicKey, error) {

	keys, err := s.client.PublicKey.FindMany(
		db.PublicKey.UserID.Equals(pg.Encode(userID.Value)),
	).Take(1000).Exec(ctx)

	if err != nil {
		return nil, err
	}

	var pbKeys []*commonpb.PublicKey
	for _, key := range keys {

		val, err := pg.Decode(key.Key)
		if err != nil {
			return nil, err
		}

		pbKeys = append(pbKeys, &commonpb.PublicKey{
			Value: val,
		})
	}

	return pbKeys, nil
}

func (s *store) RemoveKey(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) error {

	_, err := s.client.PublicKey.FindMany(
		db.PublicKey.UserID.Equals(pg.Encode(userID.Value)),
		db.PublicKey.Key.Equals(pg.Encode(pubKey.Value)),
	).Delete().Exec(ctx)

	return err
}

func (s *store) IsAuthorized(_ context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (bool, error) {

	ctx := context.Background()

	key, err := s.client.PublicKey.FindFirst(
		db.PublicKey.UserID.Equals(pg.Encode(userID.Value)),
		db.PublicKey.Key.Equals(pg.Encode(pubKey.Value)),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return key != nil, nil
}
