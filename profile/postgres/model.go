package postgres

import (
	"context"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5/pgxpool"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	profilepb "github.com/code-payments/flipchat-protobuf-api/generated/go/profile/v1"
	pg "github.com/code-payments/flipchat-server/database/postgres"

	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/flipchat-server/profile"
)

const (
	usersTableName = "flipchat_users"
	allUserFields  = `"id", "displayName", "isStaff", "isRegistered", "nextAirdropAt", "elibigibleForAirdropsUntil", "createdAt", "updatedAt"`

	xUsersTableName = "flipchat_x_users"
	allXUserFields  = `"id", "username", "name", "description", "profilePicUrl", "followerCount", "verifiedType",  "accessToken", "userId", "createdAt", "updatedAt"`
)

type xUserModel struct {
	ID            string    `db:"id"`
	Username      string    `db:"username"`
	Name          *string   `db:"name"`
	Description   *string   `db:"description"`
	ProfilePicUrl string    `db:"profilePicUrl"`
	FollowerCount int       `db:"followerCount"`
	VerifiedType  int       `db:"verifiedType"`
	AccessToken   string    `db:"accessToken"`
	UserID        string    `db:"userId"`
	CreatedAt     time.Time `db:"createdAt"`
	UpdatedAt     time.Time `db:"updatedAt"`
}

func toXUserModel(userID *commonpb.UserId, profile *profilepb.XProfile, accessToken string) (*xUserModel, error) {
	return &xUserModel{
		ID:            profile.Id,
		Username:      profile.Username,
		Name:          pointer.StringIfValid(len(profile.Name) > 0, profile.Name),
		Description:   pointer.StringIfValid(len(profile.Description) > 0, profile.Description),
		ProfilePicUrl: profile.ProfilePicUrl,
		FollowerCount: int(profile.FollowerCount),
		VerifiedType:  int(profile.VerifiedType),
		AccessToken:   accessToken,
		UserID:        pg.Encode(userID.Value),
	}, nil
}

func fromXUserModel(m *xUserModel) (*profilepb.XProfile, error) {
	return &profilepb.XProfile{
		Id:            m.ID,
		Username:      m.Username,
		Name:          *pointer.StringOrDefault(m.Name, ""),
		Description:   *pointer.StringOrDefault(m.Description, ""),
		ProfilePicUrl: m.ProfilePicUrl,
		VerifiedType:  profilepb.XProfile_VerifiedType(m.VerifiedType),
		FollowerCount: uint32(m.FollowerCount),
	}, nil
}

func dbGetDisplayName(ctx context.Context, pool *pgxpool.Pool, userID *commonpb.UserId) (*string, error) {
	var res *string
	query := `SELECT "displayName" FROM ` + usersTableName + ` WHERE "id" = $1`
	err := pgxscan.Get(
		ctx,
		pool,
		&res,
		query,
		pg.Encode(userID.Value),
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, profile.ErrNotFound
		}
		return nil, err
	}
	return res, nil
}

func dbSetDisplayName(ctx context.Context, pool *pgxpool.Pool, userID *commonpb.UserId, displayName string) error {
	query := `INSERT INTO ` + usersTableName + ` (` + allUserFields + `) VALUES ($1, $2, false, false, NOW(), NOW(), NOW(), NOW()) ON CONFLICT ("id") DO UPDATE SET "displayName" = $2 WHERE ` + usersTableName + `."id" = $1`
	_, err := pool.Exec(ctx, query, pg.Encode(userID.Value), displayName)
	return err
}

func (m *xUserModel) dbUpsert(ctx context.Context, pool *pgxpool.Pool) error {
	query := `INSERT INTO ` + xUsersTableName + ` (` + allXUserFields + `) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
		ON CONFLICT ("id") DO UPDATE
			SET "username" = $2, "name" = $3, "description" = $4, "profilePicUrl" = $5, "followerCount" = $6, "verifiedType" = $7, "accessToken" = $8, "userId" = $9, "updatedAt" = NOW()
			WHERE ` + xUsersTableName + `."id" = $1
		RETURNING ` + allXUserFields
	err := pgxscan.Get(
		ctx,
		pool,
		m,
		query,
		m.ID,
		m.Username,
		m.Name,
		m.Description,
		m.ProfilePicUrl,
		m.FollowerCount,
		m.VerifiedType,
		m.AccessToken,
		m.UserID,
	)
	return err
}

func dbUnlinkXAccount(ctx context.Context, pool *pgxpool.Pool, userID *commonpb.UserId, xUserID string) error {
	query := `DELETE FROM ` + xUsersTableName + ` WHERE "id" = $1 AND "userId" = $2`
	res, err := pool.Exec(ctx, query, xUserID, pg.Encode(userID.Value))
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return profile.ErrNotFound
	}
	return nil
}

func dbGetXUser(ctx context.Context, pool *pgxpool.Pool, userID *commonpb.UserId) (*xUserModel, error) {
	res := &xUserModel{}
	query := `SELECT ` + allXUserFields + ` FROM ` + xUsersTableName + ` WHERE "userId" = $1`
	err := pgxscan.Get(
		ctx,
		pool,
		res,
		query,
		pg.Encode(userID.Value),
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, profile.ErrNotFound
		}
		return nil, err
	}
	return res, nil
}

func dbGetUserLinkedToXAccount(ctx context.Context, pool *pgxpool.Pool, xUserID string) (*commonpb.UserId, error) {
	var encoded string
	query := `SELECT "userId" FROM ` + xUsersTableName + ` WHERE "id" = $1`
	err := pgxscan.Get(
		ctx,
		pool,
		&encoded,
		query,
		xUserID,
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, profile.ErrNotFound
		}
		return nil, err
	}
	decoded, err := pg.Decode(encoded)
	if err != nil {
		return nil, err
	}
	return &commonpb.UserId{Value: decoded}, nil
}
