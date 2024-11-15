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
	s.client.User.FindMany().Delete().Exec(context.Background())
}

func (s *store) Bind(_ context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	/*
		// in-memory implementation

		if prev, ok := m.keys[string(pubKey.Value)]; ok {
			return &commonpb.UserId{Value: []byte(prev)}, nil
		}

		keys := m.users[string(userID.Value)]
		keys = append(keys, string(pubKey.Value))
		m.users[string(userID.Value)] = keys

		m.keys[string(pubKey.Value)] = string(userID.Value)
		return proto.Clone(userID).(*commonpb.UserId), nil
	*/

	// postgres implementation

	ctx := context.Background()

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
	_, err = s.client.User.UpsertOne(
		db.User.ID.Equals(pg.Encode(userID.Value)),
	).Create(
		db.User.ID.Set(pg.Encode(userID.Value)),
	).Update().Exec(ctx)
	if err != nil {
		return nil, err
	}

	// Create a new public key if it doesn't exist
	_, err = s.client.PublicKey.UpsertOne(
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
	).Exec(ctx)

	if err != nil {
		return nil, err
	}

	return &commonpb.UserId{Value: userID.Value}, nil
}

func (s *store) GetUserId(_ context.Context, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	/*
		// in-memory implementation
		userID, ok := m.keys[string(pubKey.Value)]
		if !ok {
			return nil, account.ErrNotFound
		}

		return &commonpb.UserId{Value: []byte(userID)}, nil
	*/

	// postgres implementation

	ctx := context.Background()

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

func (s *store) GetPubKeys(_ context.Context, userID *commonpb.UserId) ([]*commonpb.PublicKey, error) {
	/*
		// in-memory implementation
		var keys []*commonpb.PublicKey
		for _, key := range m.users[string(userID.Value)] {
			keys = append(keys, &commonpb.PublicKey{Value: []byte(key)})
		}

		return keys, nil
	*/

	// postgres implementation

	ctx := context.Background()

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

func (s *store) RemoveKey(_ context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) error {
	/*
		// in-memory implementation
		boundUserID, exists := m.keys[string(pubKey.Value)]
		if !exists || boundUserID != string(userID.Value) {
			return nil
		}

		delete(m.keys, string(pubKey.Value))
		keys := m.users[string(userID.Value)]
		keys = slices.DeleteFunc(keys, func(e string) bool {
			return e == string(pubKey.Value)
		})
		m.users[string(userID.Value)] = keys
	*/

	// postgres implementation

	ctx := context.Background()

	_, err := s.client.PublicKey.FindMany(
		db.PublicKey.UserID.Equals(pg.Encode(userID.Value)),
		db.PublicKey.Key.Equals(pg.Encode(pubKey.Value)),
	).Delete().Exec(ctx)

	return err
}

func (s *store) IsAuthorized(_ context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (bool, error) {
	/*
		keys, ok := m.users[string(userID.Value)]
		if !ok {
			return false, nil
		}

		for _, key := range keys {
			if bytes.Equal([]byte(key), pubKey.Value) {
				return true, nil
			}
		}

		return false, nil
	*/

	// postgres implementation

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
