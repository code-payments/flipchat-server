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

func NewInPostgres(client *db.PrismaClient) account.Store {
	return &store{
		client,
	}
}

func (s *store) reset() {
	ctx := context.Background()

	keys := s.client.PublicKey.FindMany().Delete().Tx()
	users := s.client.User.FindMany().Delete().Tx()

	err := s.client.Prisma.Transaction(keys, users).Exec(ctx)
	if err != nil {
		panic(err)
	}
}

func (s *store) Bind(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	encodedUserID := pg.Encode(userID.Value)
	encodedPubKey := pg.Encode(pubKey.Value, pg.Base58)

	// Check if this pubkey is already bound to a user
	key, err := s.client.PublicKey.FindUnique(
		db.PublicKey.Key.Equals(encodedPubKey),
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
		db.User.ID.Equals(encodedUserID),
	).Create(
		db.User.ID.Set(encodedUserID),
	).Update().Tx()

	// Create a new public key if it doesn't exist
	keyTx := s.client.PublicKey.UpsertOne(
		db.PublicKey.Key.Equals(encodedPubKey),
	).Create(
		db.PublicKey.Key.Set(encodedPubKey),
		db.PublicKey.User.Link(
			db.User.ID.Equals(encodedUserID),
		),
	).Update(
		db.PublicKey.User.Link(
			db.User.ID.Equals(encodedUserID),
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
	encodedPubKey := pg.Encode(pubKey.Value, pg.Base58)

	key, err := s.client.PublicKey.FindFirst(
		db.PublicKey.Key.Equals(encodedPubKey),
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
	encodedUserID := pg.Encode(userID.Value)

	// TODO: Add pagination
	keys, err := s.client.PublicKey.FindMany(
		db.PublicKey.UserID.Equals(encodedUserID),
	).Exec(ctx)

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
	encodedUserID := pg.Encode(userID.Value)
	encodedPubKey := pg.Encode(pubKey.Value, pg.Base58)

	_, err := s.client.PublicKey.FindMany(
		db.PublicKey.UserID.Equals(encodedUserID),
		db.PublicKey.Key.Equals(encodedPubKey),
	).Delete().Exec(ctx)

	return err
}

func (s *store) IsAuthorized(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (bool, error) {
	encodedUserID := pg.Encode(userID.Value)
	encodedPubKey := pg.Encode(pubKey.Value, pg.Base58)

	key, err := s.client.PublicKey.FindFirst(
		db.PublicKey.UserID.Equals(encodedUserID),
		db.PublicKey.Key.Equals(encodedPubKey),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return key != nil, nil
}
