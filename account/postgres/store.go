package postgres

import (
	"bytes"
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/account"
)

type store struct {
	pool *pgxpool.Pool
}

func NewInPostgres(pool *pgxpool.Pool) account.Store {
	return &store{
		pool: pool,
	}
}

func (s *store) Bind(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	existingUserID, err := dbGetUserId(ctx, s.pool, pubKey)
	if err != nil && err != account.ErrNotFound {
		return nil, err
	} else if err == nil {
		return existingUserID, nil
	}

	err = dbBind(ctx, s.pool, userID, pubKey)
	if err != nil {
		return nil, err
	}
	return &commonpb.UserId{Value: userID.Value}, nil
}

func (s *store) GetUserId(ctx context.Context, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	return dbGetUserId(ctx, s.pool, pubKey)
}

func (s *store) GetPubKeys(ctx context.Context, userID *commonpb.UserId) ([]*commonpb.PublicKey, error) {
	return dbGetPubKeys(ctx, s.pool, userID)
}

func (s *store) RemoveKey(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) error {
	return dbRemoveKey(ctx, s.pool, userID, pubKey)
}

func (s *store) IsAuthorized(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (bool, error) {
	linkedUserID, err := dbGetUserId(ctx, s.pool, pubKey)
	if err == account.ErrNotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return bytes.Equal(linkedUserID.Value, userID.Value), nil
}

func (s *store) IsStaff(ctx context.Context, userID *commonpb.UserId) (bool, error) {
	return dbIsStaff(ctx, s.pool, userID)
}

func (s *store) IsRegistered(ctx context.Context, userID *commonpb.UserId) (bool, error) {
	return dbIsRegistered(ctx, s.pool, userID)
}

func (s *store) SetRegistrationFlag(ctx context.Context, userID *commonpb.UserId, isRegistered bool) error {
	return dbSetRegistrationFlag(ctx, s.pool, userID, isRegistered)
}

func (s *store) BatchSetNextAirdropTimestamp(ctx context.Context, ts time.Time, userIDs ...*commonpb.UserId) error {
	return dbBatchSetNextAirdropTimestamp(ctx, s.pool, ts, userIDs...)
}

func (s *store) GetNextAirdropTimestamp(ctx context.Context, userID *commonpb.UserId) (time.Time, error) {
	return dbGetNextAirdropTimestamp(ctx, s.pool, userID)
}

func (s *store) ExtendAirdropEligibility(ctx context.Context, userID *commonpb.UserId, until time.Time) error {
	return dbExtendAirdropEligibility(ctx, s.pool, userID, until)
}

func (s *store) GetAirdropEligibilityTimestamp(ctx context.Context, userID *commonpb.UserId) (time.Time, error) {
	return dbGetAirdropEligibilityTimestamp(ctx, s.pool, userID)
}

func (s *store) GetUsersToAirdrop(ctx context.Context, at time.Time) ([]*commonpb.UserId, error) {
	return dbGetUsersToAirdrop(ctx, s.pool, at)
}

func (s *store) reset() {
	_, err := s.pool.Exec(context.Background(), "DELETE FROM "+publicKeysTableName)
	if err != nil {
		panic(err)
	}

	_, err = s.pool.Exec(context.Background(), "DELETE FROM "+usersTableName)
	if err != nil {
		panic(err)
	}
}
