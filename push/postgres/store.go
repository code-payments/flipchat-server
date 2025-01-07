package postgres

import (
	"context"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	pushpb "github.com/code-payments/flipchat-protobuf-api/generated/go/push/v1"
	pg "github.com/code-payments/flipchat-server/database/postgres"

	"github.com/code-payments/flipchat-server/database/prisma/db"
	"github.com/code-payments/flipchat-server/push"
)

type store struct {
	client *db.PrismaClient
}

func NewInPostgres(client *db.PrismaClient) push.TokenStore {
	return &store{
		client,
	}
}

func (s *store) reset() {
	ctx := context.Background()

	tokens := s.client.PushToken.FindMany().Delete().Tx()

	err := s.client.Prisma.Transaction(tokens).Exec(ctx)
	if err != nil {
		panic(err)
	}
}

func (s *store) GetTokens(ctx context.Context, userID *commonpb.UserId) ([]push.Token, error) {

	encodedUserID := pg.Encode(userID.Value)

	tokens, err := s.client.PushToken.FindMany(
		db.PushToken.UserID.Equals(encodedUserID),
	).Exec(ctx)

	if err != nil {
		return nil, err
	}

	res := make([]push.Token, len(tokens))
	for i, token := range tokens {
		res[i] = push.Token{
			Type:         pushpb.TokenType(token.Type),
			Token:        token.Token,
			AppInstallID: token.AppInstallID,
		}
	}

	return res, nil
}

func (s *store) GetTokensBatch(ctx context.Context, userIDs ...*commonpb.UserId) ([]push.Token, error) {

	encodedUserIDs := make([]string, len(userIDs))
	for i, userID := range userIDs {
		encodedUserIDs[i] = pg.Encode(userID.Value)
	}

	tokens, err := s.client.PushToken.FindMany(
		db.PushToken.UserID.In(encodedUserIDs),
	).Exec(ctx)

	if err != nil {
		return nil, err
	}

	res := make([]push.Token, len(tokens))
	for i, token := range tokens {
		res[i] = push.Token{
			Type:         pushpb.TokenType(token.Type),
			Token:        token.Token,
			AppInstallID: token.AppInstallID,
		}
	}

	return res, nil
}

func (s *store) AddToken(ctx context.Context, userID *commonpb.UserId, appInstallID *commonpb.AppInstallId, tokenType pushpb.TokenType, token string) error {

	encodedUserID := pg.Encode(userID.Value)

	_, err := s.client.PushToken.UpsertOne(
		db.PushToken.UserIDAppInstallID(
			db.PushToken.UserID.Equals(encodedUserID),
			db.PushToken.AppInstallID.Equals(appInstallID.Value),
		),
	).Create(
		db.PushToken.UserID.Set(encodedUserID),
		db.PushToken.AppInstallID.Set(appInstallID.Value),
		db.PushToken.Token.Set(token),
		db.PushToken.Type.Set(int(tokenType)),
	).Update(
		db.PushToken.Token.Set(token),
	).Exec(ctx)

	if err != nil {
		return err
	}

	return nil
}

func (s *store) DeleteToken(ctx context.Context, tokenType pushpb.TokenType, token string) error {

	_, err := s.client.PushToken.FindMany(
		db.PushToken.Token.Equals(token),
		db.PushToken.Type.Equals(int(tokenType)),
	).Delete().Exec(ctx)

	return err
}

func (s *store) ClearTokens(_ context.Context, userID *commonpb.UserId) error {

	encodedUserID := pg.Encode(userID.Value)

	_, err := s.client.PushToken.FindMany(
		db.PushToken.UserID.Equals(encodedUserID),
	).Delete().Exec(context.Background())

	return err
}
