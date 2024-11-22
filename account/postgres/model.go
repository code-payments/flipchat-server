package postgres

import (
	"database/sql"
	"time"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	pgutil "github.com/code-payments/flipchat-server/database/postgres"
)

const (
	userTable      = "flipchat_users"
	publicKeyTable = "flipchat_publickeys"
)

// userModel maps to the flipchat_users table
type userModel struct {
	ID          string         `db:"id"`
	DisplayName sql.NullString `db:"displayName"`
	CreatedAt   time.Time      `db:"createdAt"`
	UpdatedAt   time.Time      `db:"updatedAt"`
}

// publicKeyModel maps to the flipchat_publickeys table
type publicKeyModel struct {
	Key       string    `db:"key"`
	UserID    string    `db:"userId"`
	CreatedAt time.Time `db:"createdAt"`
	UpdatedAt time.Time `db:"updatedAt"`
}

// toUserModel converts a commonpb.UserId to a userModel
func toUserModel(userID *commonpb.UserId, displayName *string) *userModel {
	return &userModel{
		ID:          pgutil.Encode(userID.Value),
		DisplayName: pgutil.NullString(displayName),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// fromUserModel converts a userModel to a commonpb.UserId
func fromUserModel(m *userModel) (*commonpb.UserId, error) {
	userID, err := pgutil.Decode(m.ID)
	if err != nil {
		return nil, err
	}
	return &commonpb.UserId{Value: userID}, nil
}

// toPublicKeyModel converts a commonpb.PublicKey to a publicKeyModel
func toPublicKeyModel(userID *commonpb.UserId, pubKey *commonpb.PublicKey) *publicKeyModel {
	return &publicKeyModel{
		Key:       pgutil.Encode(pubKey.Value, pgutil.Base58),
		UserID:    pgutil.Encode(userID.Value),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// fromPublicKeyModel converts a publicKeyModel to a commonpb.PublicKey
func fromPublicKeyModel(m *publicKeyModel) (*commonpb.PublicKey, error) {
	key, err := pgutil.Decode(m.Key)
	if err != nil {
		return nil, err
	}
	return &commonpb.PublicKey{Value: key}, nil
}
