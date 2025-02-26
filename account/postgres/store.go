package postgres

import (
	"context"
	"errors"
	"time"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	pg "github.com/code-payments/flipchat-server/database/postgres"

	"github.com/code-payments/code-server/pkg/metrics"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/database/prisma/db"
)

const (
	metricsStructName = "account.postgres.store"
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
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "Bind")
	defer tracer.End()

	res, err := func() (*commonpb.UserId, error) {
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
	}()

	tracer.OnError(err)

	return res, err
}

func (s *store) GetUserId(ctx context.Context, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetUserID")
	defer tracer.End()

	res, err := func() (*commonpb.UserId, error) {
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
	}()

	tracer.OnError(err)

	return res, err
}

func (s *store) GetPubKeys(ctx context.Context, userID *commonpb.UserId) ([]*commonpb.PublicKey, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetPubKeys")
	defer tracer.End()

	res, err := func() ([]*commonpb.PublicKey, error) {
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
	}()

	tracer.OnError(err)

	return res, err
}

func (s *store) RemoveKey(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) error {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "RemoveKey")
	defer tracer.End()

	err := func() error {
		encodedUserID := pg.Encode(userID.Value)
		encodedPubKey := pg.Encode(pubKey.Value, pg.Base58)

		_, err := s.client.PublicKey.FindMany(
			db.PublicKey.UserID.Equals(encodedUserID),
			db.PublicKey.Key.Equals(encodedPubKey),
		).Delete().Exec(ctx)

		return err
	}()

	tracer.OnError(err)

	return err
}

func (s *store) IsAuthorized(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "IsAuthorized")
	defer tracer.End()

	res, err := func() (bool, error) {
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
	}()

	tracer.OnError(err)

	return res, err
}

func (s *store) IsStaff(ctx context.Context, userID *commonpb.UserId) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "IsStaff")
	defer tracer.End()

	res, err := func() (bool, error) {
		encodedUserID := pg.Encode(userID.Value)

		res, err := s.client.User.FindUnique(
			db.User.ID.Equals(encodedUserID),
		).Exec(ctx)

		if errors.Is(err, db.ErrNotFound) {
			return false, nil
		}

		if err != nil {
			return false, err
		}

		return res.IsStaff, nil
	}()

	tracer.OnError(err)

	return res, err
}

func (s *store) IsRegistered(ctx context.Context, userID *commonpb.UserId) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "IsRegistered")
	defer tracer.End()

	res, err := func() (bool, error) {
		encodedUserID := pg.Encode(userID.Value)

		res, err := s.client.User.FindUnique(
			db.User.ID.Equals(encodedUserID),
		).Exec(ctx)

		if errors.Is(err, db.ErrNotFound) {
			return false, nil
		}

		if err != nil {
			return false, err
		}

		return res.IsRegistered, nil
	}()

	tracer.OnError(err)

	return res, err
}

func (s *store) SetRegistrationFlag(ctx context.Context, userID *commonpb.UserId, isRegistered bool) error {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "SetRegistrationFlag")
	defer tracer.End()

	err := func() error {
		encodedUserID := pg.Encode(userID.Value)

		_, err := s.client.User.FindUnique(
			db.User.ID.Equals(encodedUserID),
		).Update(
			db.User.IsRegistered.Set(isRegistered),
		).Exec(ctx)

		if errors.Is(err, db.ErrNotFound) {
			return account.ErrNotFound
		}

		return err
	}()

	tracer.OnError(err)

	return err
}

func (s *store) SetNextAirdropTimestamp(ctx context.Context, userID *commonpb.UserId, ts time.Time) error {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "SetRegistrationFlag")
	defer tracer.End()

	err := func() error {
		encodedUserID := pg.Encode(userID.Value)

		_, err := s.client.User.FindUnique(
			db.User.ID.Equals(encodedUserID),
		).Update(
			db.User.NextAirdropAt.Set(ts),
		).Exec(ctx)

		if errors.Is(err, db.ErrNotFound) {
			return account.ErrNotFound
		}

		return err
	}()

	tracer.OnError(err)

	return err
}

func (s *store) GetNextAirdropTimestamp(ctx context.Context, userID *commonpb.UserId) (time.Time, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetNextAirdropTimestamp")
	defer tracer.End()

	res, err := func() (time.Time, error) {
		encodedUserID := pg.Encode(userID.Value)

		res, err := s.client.User.FindUnique(
			db.User.ID.Equals(encodedUserID),
		).Exec(ctx)

		if errors.Is(err, db.ErrNotFound) {
			return time.Time{}, account.ErrNotFound
		} else if err != nil {
			return time.Time{}, err
		}

		return res.NextAirdropAt, nil
	}()

	tracer.OnError(err)

	return res, err
}

func (s *store) ExtendAirdropEligibility(ctx context.Context, userID *commonpb.UserId, until time.Time) error {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "ExtendAirdropEligibility")
	defer tracer.End()

	err := func() error {
		encodedUserID := pg.Encode(userID.Value)

		_, err := s.client.User.FindUnique(
			db.User.ID.Equals(encodedUserID),
		).Update(
			db.User.ElibigibleForAirdropsUntil.Set(until),
		).Exec(ctx)

		if errors.Is(err, db.ErrNotFound) {
			return account.ErrNotFound
		}

		return err
	}()

	tracer.OnError(err)

	return err
}

func (s *store) GetAirdropEligibilityTimestamp(ctx context.Context, userID *commonpb.UserId) (time.Time, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetAirdropEligibilityTimestamp")
	defer tracer.End()

	res, err := func() (time.Time, error) {
		encodedUserID := pg.Encode(userID.Value)

		res, err := s.client.User.FindUnique(
			db.User.ID.Equals(encodedUserID),
		).Exec(ctx)

		if errors.Is(err, db.ErrNotFound) {
			return time.Time{}, account.ErrNotFound
		} else if err != nil {
			return time.Time{}, err
		}

		return res.ElibigibleForAirdropsUntil, nil
	}()

	tracer.OnError(err)

	return res, err
}

func (s *store) GetUsersToAirdrop(ctx context.Context, at time.Time) ([]*commonpb.UserId, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetUsersToAirdrop")
	defer tracer.End()

	res, err := func() ([]*commonpb.UserId, error) {

		res, err := s.client.User.FindMany(
			db.User.NextAirdropAt.Before(at),
			db.User.ElibigibleForAirdropsUntil.After(at),
			db.User.IsRegistered.Equals(true),
		).Exec(ctx)

		if errors.Is(err, db.ErrNotFound) {
			return nil, account.ErrNotFound
		} else if err != nil {
			return nil, err
		}

		var userIDs []*commonpb.UserId
		for _, user := range res {
			val, err := pg.Decode(user.ID)
			if err != nil {
				return nil, err
			}

			userIDs = append(userIDs, &commonpb.UserId{Value: val})
		}

		if len(userIDs) == 0 {
			return nil, account.ErrNotFound
		}
		return userIDs, nil
	}()

	tracer.OnError(err)

	return res, err
}

func (s *store) GetCreationTimestamp(ctx context.Context, userID *commonpb.UserId) (time.Time, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetCreationTimestamp")
	defer tracer.End()

	res, err := func() (time.Time, error) {
		encodedUserID := pg.Encode(userID.Value)

		res, err := s.client.User.FindUnique(
			db.User.ID.Equals(encodedUserID),
		).Exec(ctx)

		if errors.Is(err, db.ErrNotFound) {
			return time.Time{}, account.ErrNotFound
		} else if err != nil {
			return time.Time{}, err
		}

		return res.CreatedAt, nil
	}()

	tracer.OnError(err)

	return res, err
}
