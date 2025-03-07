package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5/pgxpool"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	"github.com/code-payments/flipchat-server/account"
	pg "github.com/code-payments/flipchat-server/database/postgres"
)

const (
	publicKeysTableName = "flipchat_publickeys"
	allPublicKeyFields  = `"key", "userId", "createdAt", "updatedAt"`

	usersTableName = "flipchat_users"
	allUserFields  = `"id", "displayName", "isStaff", "isRegistered", "nextAirdropAt", "elibigibleForAirdropsUntil", "createdAt", "updatedAt"`
)

func dbBind(ctx context.Context, pool *pgxpool.Pool, userID *commonpb.UserId, pubKey *commonpb.PublicKey) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}

	upsertUserQuery := `INSERT INTO ` + usersTableName + ` (` + allUserFields + `) VALUES ($1, NULL, false, false, NOW(), NOW(), NOW(), NOW()) ON CONFLICT ("id") DO NOTHING`
	_, err = tx.Exec(ctx, upsertUserQuery, pg.Encode(userID.Value))
	if err != nil {
		return err
	}

	putPubkeyQuery := `INSERT INTO ` + publicKeysTableName + ` (` + allPublicKeyFields + `) VALUES ($1, $2, NOW(), NOW()) ON CONFLICT ("key") DO UPDATE SET "userId" = $2 WHERE ` + publicKeysTableName + `."key" = $1`
	_, err = tx.Exec(ctx, putPubkeyQuery, pg.Encode(pubKey.Value, pg.Base58), pg.Encode(userID.Value))
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func dbGetUserId(ctx context.Context, pool *pgxpool.Pool, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	var encoded string
	query := `SELECT "userId" FROM ` + publicKeysTableName + ` WHERE "key" = $1`
	err := pgxscan.Get(
		ctx,
		pool,
		&encoded,
		query,
		pg.Encode(pubKey.Value, pg.Base58),
	)
	if pgxscan.NotFound(err) {
		return nil, account.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	decoded, err := pg.Decode(encoded)
	if err != nil {
		return nil, err
	}
	return &commonpb.UserId{Value: decoded}, err
}

func dbGetPubKeys(ctx context.Context, pool *pgxpool.Pool, userID *commonpb.UserId) ([]*commonpb.PublicKey, error) {
	var encodedValues []string
	query := `SELECT "key" FROM ` + publicKeysTableName + ` WHERE "userId" = $1`
	err := pgxscan.Select(
		ctx,
		pool,
		&encodedValues,
		query,
		pg.Encode(userID.Value),
	)
	if pgxscan.NotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if len(encodedValues) == 0 {
		return nil, nil
	}
	res := make([]*commonpb.PublicKey, len(encodedValues))
	for i, encodedValue := range encodedValues {
		decodedValue, err := pg.Decode(encodedValue)
		if err != nil {
			return nil, err
		}
		res[i] = &commonpb.PublicKey{Value: decodedValue}
	}
	return res, nil
}

func dbRemoveKey(ctx context.Context, pool *pgxpool.Pool, userID *commonpb.UserId, pubKey *commonpb.PublicKey) error {
	query := `DELETE FROM ` + publicKeysTableName + ` WHERE "key" = $1 AND "userId" = $2`
	_, err := pool.Exec(ctx, query, pg.Encode(pubKey.Value, pg.Base58), pg.Encode(userID.Value))
	return err
}

func dbIsStaff(ctx context.Context, pool *pgxpool.Pool, userID *commonpb.UserId) (bool, error) {
	var res bool
	query := `SELECT "isStaff" FROM ` + usersTableName + ` WHERE "id" = $1`
	err := pgxscan.Get(
		ctx,
		pool,
		&res,
		query,
		pg.Encode(userID.Value),
	)
	if pgxscan.NotFound(err) {
		return false, nil
	}
	return res, err
}

func dbIsRegistered(ctx context.Context, pool *pgxpool.Pool, userID *commonpb.UserId) (bool, error) {
	var res bool
	query := `SELECT "isRegistered" FROM ` + usersTableName + ` WHERE "id" = $1`
	err := pgxscan.Get(
		ctx,
		pool,
		&res,
		query,
		pg.Encode(userID.Value),
	)
	if pgxscan.NotFound(err) {
		return false, nil
	}
	return res, err
}

func dbSetRegistrationFlag(ctx context.Context, pool *pgxpool.Pool, userID *commonpb.UserId, isRegistered bool) error {
	query := `UPDATE ` + usersTableName + ` SET "isRegistered" = $1 WHERE "id" = $2`
	res, err := pool.Exec(ctx, query, isRegistered, pg.Encode(userID.Value))
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return account.ErrNotFound
	}
	return nil
}

func dbBatchSetNextAirdropTimestamp(ctx context.Context, pool *pgxpool.Pool, ts time.Time, userIDs ...*commonpb.UserId) error {
	queryParameters := make([]any, len(userIDs)+1)
	queryParameters[0] = ts.UTC()
	query := `UPDATE ` + usersTableName + ` SET "nextAirdropAt" = $1 WHERE "id" IN (`
	for i, userID := range userIDs {
		queryParameters[i+1] = pg.Encode(userID.Value)
		if i > 0 {
			query += fmt.Sprintf(",$%d", i+2)
		} else {
			query += fmt.Sprintf("$%d", i+2)
		}
	}
	query += `)`
	_, err := pool.Exec(ctx, query, queryParameters...)
	if err != nil {
		return err
	}
	return err
}

func dbGetNextAirdropTimestamp(ctx context.Context, pool *pgxpool.Pool, userID *commonpb.UserId) (time.Time, error) {
	res := struct {
		NextAirdropAt time.Time `db:"nextAirdropAt"`
	}{}
	query := `SELECT "nextAirdropAt" FROM ` + usersTableName + ` WHERE "id" = $1`
	err := pgxscan.Get(
		ctx,
		pool,
		&res,
		query,
		pg.Encode(userID.Value),
	)
	if pgxscan.NotFound(err) {
		return time.Time{}, account.ErrNotFound
	} else if err != nil {
		return time.Time{}, err
	}
	return res.NextAirdropAt, err
}

func dbExtendAirdropEligibility(ctx context.Context, pool *pgxpool.Pool, userID *commonpb.UserId, until time.Time) error {
	query := `UPDATE ` + usersTableName + ` SET "elibigibleForAirdropsUntil" = $1 WHERE "id" = $2`
	res, err := pool.Exec(ctx, query, until.UTC(), pg.Encode(userID.Value))
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return account.ErrNotFound
	}
	return nil
}

func dbGetAirdropEligibilityTimestamp(ctx context.Context, pool *pgxpool.Pool, userID *commonpb.UserId) (time.Time, error) {
	res := struct {
		ElibigibleForAirdropsUntil time.Time `db:"elibigibleForAirdropsUntil"`
	}{}
	query := `SELECT "elibigibleForAirdropsUntil" FROM ` + usersTableName + ` WHERE "id" = $1`
	err := pgxscan.Get(
		ctx,
		pool,
		&res,
		query,
		pg.Encode(userID.Value),
	)
	if pgxscan.NotFound(err) {
		return time.Time{}, account.ErrNotFound
	} else if err != nil {
		return time.Time{}, err
	}
	return res.ElibigibleForAirdropsUntil, err
}

func dbGetUsersToAirdrop(ctx context.Context, pool *pgxpool.Pool, at time.Time) ([]*commonpb.UserId, error) {
	var encodedValues []string
	query := `SELECT "id" FROM ` + usersTableName + ` WHERE "isRegistered" = true AND "nextAirdropAt" < $1 AND "elibigibleForAirdropsUntil" > $1`
	err := pgxscan.Select(
		ctx,
		pool,
		&encodedValues,
		query,
		at.UTC(),
	)
	if pgxscan.NotFound(err) {
		return nil, account.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if len(encodedValues) == 0 {
		return nil, account.ErrNotFound
	}
	res := make([]*commonpb.UserId, len(encodedValues))
	for i, encodedValue := range encodedValues {
		decodedValue, err := pg.Decode(encodedValue)
		if err != nil {
			return nil, err
		}
		res[i] = &commonpb.UserId{Value: decodedValue}
	}
	return res, nil
}
