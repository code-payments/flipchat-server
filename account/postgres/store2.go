package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	pg "github.com/code-payments/flipchat-server/database/postgres"

	"github.com/code-payments/flipchat-server/account"
)

type pgStore struct {
	db *sqlx.DB
}

func NewInPostgres2(db *sql.DB) account.Store {
	return &pgStore{
		db: sqlx.NewDb(db, "pgx"),
	}
}

func (s *pgStore) reset() {
	ctx := context.Background()

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		panic(err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		} else if err != nil {
			_ = tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	// Delete all public keys
	_, err = tx.ExecContext(ctx, `DELETE FROM `+publicKeyTable)
	if err != nil {
		panic(err)
	}

	// Delete all users
	_, err = tx.ExecContext(ctx, `DELETE FROM `+userTable)
	if err != nil {
		panic(err)
	}
}

func (s *pgStore) Bind(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	encodedUserID := pg.Encode(userID.Value)
	encodedPubKey := pg.Encode(pubKey.Value, pg.Base58)

	// Check if this pubkey is already bound to a user
	var existingPubKey publicKeyModel
	query := `SELECT "key", "userId" FROM ` + publicKeyTable + ` WHERE "key" = $1`
	err := s.db.GetContext(ctx, &existingPubKey, query, encodedPubKey)

	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if err == nil {
		// Public key is already bound
		val, err := pg.Decode(existingPubKey.UserID)
		if err != nil {
			return nil, err
		}

		// Cannot rebind without revoking first
		return &commonpb.UserId{Value: val}, nil
	}

	// Start a transaction
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // re-throw panic after Rollback
		} else if err != nil {
			_ = tx.Rollback() // err is non-nil; rollback
		} else {
			err = tx.Commit() // err is nil; if Commit returns error update err
		}
	}()

	currentTime := time.Now()

	// Upsert user
	_, err = tx.ExecContext(ctx, `
		INSERT INTO `+userTable+` ("id", "createdAt", "updatedAt")
		VALUES ($1, $2, $3)
		ON CONFLICT ("id") DO NOTHING
	`, encodedUserID, currentTime, currentTime)
	if err != nil {
		return nil, err
	}

	// Upsert public key
	_, err = tx.ExecContext(ctx, `
		INSERT INTO `+publicKeyTable+` ("key", "userId", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4)
		ON CONFLICT ("key") DO UPDATE SET "userId" = EXCLUDED."userId", "updatedAt" = EXCLUDED."updatedAt"
	`, encodedPubKey, encodedUserID, currentTime, currentTime)
	if err != nil {
		return nil, err
	}

	// The transaction will be committed by the deferred function if err == nil

	return &commonpb.UserId{Value: userID.Value}, nil
}

func (s *pgStore) GetUserId(ctx context.Context, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	encodedPubKey := pg.Encode(pubKey.Value, pg.Base58)

	var pubKeyM publicKeyModel
	query := `SELECT "key", "userId" FROM ` + publicKeyTable + ` WHERE "key" = $1`
	err := s.db.GetContext(ctx, &pubKeyM, query, encodedPubKey)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, account.ErrNotFound
		}
		return nil, err
	}

	userID, err := pg.Decode(pubKeyM.UserID)
	if err != nil {
		return nil, err
	}

	return &commonpb.UserId{Value: userID}, nil
}

func (s *pgStore) GetPubKeys(ctx context.Context, userID *commonpb.UserId) ([]*commonpb.PublicKey, error) {
	encodedUserID := pg.Encode(userID.Value)

	var pubKeys []publicKeyModel
	query := `SELECT "key" FROM ` + publicKeyTable + ` WHERE "userId" = $1`
	err := s.db.SelectContext(ctx, &pubKeys, query, encodedUserID)
	if err != nil {
		return nil, err
	}
	if len(pubKeys) == 0 {
		return nil, account.ErrNotFound
	}

	var result []*commonpb.PublicKey
	for _, pk := range pubKeys {
		key, err := pg.Decode(pk.Key)
		if err != nil {
			return nil, err
		}
		result = append(result, &commonpb.PublicKey{Value: key})
	}
	return result, nil
}

func (s *pgStore) RemoveKey(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) error {
	encodedUserID := pg.Encode(userID.Value)
	encodedPubKey := pg.Encode(pubKey.Value, pg.Base58)

	query := `DELETE FROM ` + publicKeyTable + ` WHERE "key" = $1 AND "userId" = $2`
	result, err := s.db.ExecContext(ctx, query, encodedPubKey, encodedUserID)
	if err != nil {
		return err
	}
	_, err = result.RowsAffected()
	if err != nil {
		return err
	}

	return nil
}

func (s *pgStore) IsAuthorized(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (bool, error) {
	encodedUserID := pg.Encode(userID.Value)
	encodedPubKey := pg.Encode(pubKey.Value, pg.Base58)

	var count int
	query := `SELECT COUNT(*) FROM ` + publicKeyTable + ` WHERE "key" = $1 AND "userId" = $2`
	err := s.db.GetContext(ctx, &count, query, encodedPubKey, encodedUserID)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}
