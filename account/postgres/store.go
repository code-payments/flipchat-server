package postgres

import (
	"context"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

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

func (pg *store) reset() {
}

func (pg *store) Bind(_ context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	panic("implement me")
}

func (pg *store) GetUserId(_ context.Context, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	panic("implement me")
}

func (pg *store) GetPubKeys(_ context.Context, userID *commonpb.UserId) ([]*commonpb.PublicKey, error) {
	panic("implement me")
}

func (pg *store) RemoveKey(_ context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) error {
	panic("implement me")
}

func (pg *store) IsAuthorized(_ context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (bool, error) {
	panic("implement me")
}
