package postgres

import (
	"context"
	"errors"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	profilepb "github.com/code-payments/flipchat-protobuf-api/generated/go/profile/v1"
	pg "github.com/code-payments/flipchat-server/database/postgres"

	"github.com/code-payments/flipchat-server/database/prisma/db"
	"github.com/code-payments/flipchat-server/profile"
)

type store struct {
	client *db.PrismaClient
}

func NewInPostgres(client *db.PrismaClient) profile.Store {
	return &store{
		client,
	}
}

func (s *store) reset() {
	ctx := context.Background()

	users := s.client.User.FindMany().Update(db.User.DisplayName.SetOptional(nil)).Tx()

	err := s.client.Prisma.Transaction(users).Exec(ctx)
	if err != nil {
		panic(err)
	}
}

func (s *store) GetProfile(ctx context.Context, id *commonpb.UserId) (*profilepb.UserProfile, error) {
	encodedUserID := pg.Encode(id.Value)

	baseProfile, err := s.client.User.FindFirst(
		db.User.ID.Equals(encodedUserID),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) {
		return nil, profile.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	// User found but the optional display name is not set
	name, ok := baseProfile.DisplayName()
	if !ok {
		return nil, profile.ErrNotFound
	}

	var socialProfiles []*profilepb.SocialProfile

	xProfile, err := s.client.XUser.FindFirst(
		db.XUser.UserID.Equals(encodedUserID),
	).Exec(ctx)

	if err == nil {
		protoXProfile, err := fromXUserModel(xProfile)
		if err != nil {
			return nil, err
		}
		socialProfiles = append(socialProfiles, &profilepb.SocialProfile{
			Type: &profilepb.SocialProfile_X{
				X: protoXProfile,
			},
		})
	} else if !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}

	return &profilepb.UserProfile{
		DisplayName:    name,
		SocialProfiles: socialProfiles,
	}, nil
}

func (s *store) SetDisplayName(ctx context.Context, id *commonpb.UserId, displayName string) error {
	encodedUserID := pg.Encode(id.Value)

	// TODO: The upsert feels a bit weird here but the memory store does it too

	// Upsert the user with the new display name
	_, err := s.client.User.UpsertOne(
		db.User.ID.Equals(encodedUserID),
	).Create(
		db.User.ID.Set(encodedUserID),
		db.User.DisplayName.Set(displayName),
	).Update(
		db.User.DisplayName.Set(displayName),
	).Exec(ctx)

	return err
}

func (s *store) LinkXAccount(ctx context.Context, userID *commonpb.UserId, xProfile *profilepb.XProfile, accessToken string) error {
	encodedUserID := pg.Encode(userID.Value)

	existing, err := s.client.XUser.FindUnique(db.XUser.UserID.Equals(encodedUserID)).Exec(ctx)
	if err == nil {
		if existing.ID == xProfile.Id {
			// todo: Upsert doesn't update fields if it's the same user
			_, err = s.client.XUser.FindUnique(db.XUser.ID.Equals(xProfile.Id)).
				Update(
					db.XUser.Username.Set(xProfile.Username),
					db.XUser.Name.Set(xProfile.Name),
					db.XUser.Description.Set(xProfile.Description),
					db.XUser.ProfilePicURL.Set(xProfile.ProfilePicUrl),
					db.XUser.AccessToken.Set(accessToken),
					db.XUser.FollowerCount.Set(int(xProfile.FollowerCount)),
					db.XUser.VerifiedType.Set(int(xProfile.VerifiedType)),
				).
				Exec(ctx)
			return err
		}
		return profile.ErrExistingSocialLink
	} else if !errors.Is(err, db.ErrNotFound) {
		return err
	}

	_, err = s.client.XUser.UpsertOne(db.XUser.ID.Equals(xProfile.Id)).
		Create(
			db.XUser.ID.Set(xProfile.Id),
			db.XUser.Username.Set(xProfile.Username),
			db.XUser.Name.Set(xProfile.Name),
			db.XUser.Description.Set(xProfile.Description),
			db.XUser.ProfilePicURL.Set(xProfile.ProfilePicUrl),
			db.XUser.AccessToken.Set(accessToken),
			db.XUser.User.Link(
				db.User.ID.Equals(encodedUserID),
			),
			db.XUser.FollowerCount.Set(int(xProfile.FollowerCount)),
			db.XUser.VerifiedType.Set(int(xProfile.VerifiedType)),
		).
		Update(
			db.XUser.Username.Set(xProfile.Username),
			db.XUser.Name.Set(xProfile.Name),
			db.XUser.Description.Set(xProfile.Description),
			db.XUser.ProfilePicURL.Set(xProfile.ProfilePicUrl),
			db.XUser.AccessToken.Set(accessToken),
			db.XUser.User.Link(
				db.User.ID.Equals(encodedUserID),
			),
			db.XUser.FollowerCount.Set(int(xProfile.FollowerCount)),
			db.XUser.VerifiedType.Set(int(xProfile.VerifiedType)),
		).
		Exec(ctx)
	return err
}

func (s *store) GetXProfile(ctx context.Context, userID *commonpb.UserId) (*profilepb.XProfile, error) {
	encodedUserID := pg.Encode(userID.Value)

	res, err := s.client.XUser.FindUnique(db.XUser.UserID.Equals(encodedUserID)).Exec(ctx)
	if errors.Is(err, db.ErrNotFound) {
		return nil, profile.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	return fromXUserModel(res)
}

func (s *store) GetUserLinkedToXAccount(ctx context.Context, xUserID string) (*commonpb.UserId, error) {
	res, err := s.client.XUser.FindUnique(db.XUser.ID.Equals(xUserID)).Exec(ctx)
	if errors.Is(err, db.ErrNotFound) {
		return nil, profile.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	decodedUserID, err := pg.Decode(res.UserID)
	if err != nil {
		return nil, err
	}
	return &commonpb.UserId{Value: decodedUserID}, nil
}

func fromXUserModel(m *db.XUserModel) (*profilepb.XProfile, error) {
	return &profilepb.XProfile{
		Id:            m.ID,
		Username:      m.Username,
		Name:          m.Name,
		Description:   m.Description,
		ProfilePicUrl: m.ProfilePicURL,
		VerifiedType:  profilepb.XProfile_VerifiedType(m.VerifiedType),
		FollowerCount: uint32(m.FollowerCount),
	}, nil
}
