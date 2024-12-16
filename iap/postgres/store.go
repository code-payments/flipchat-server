package postgres

import (
	"context"
	"errors"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	pg "github.com/code-payments/flipchat-server/database/postgres"
	"github.com/code-payments/flipchat-server/database/prisma/db"
	"github.com/code-payments/flipchat-server/iap"
)

type store struct {
	client *db.PrismaClient
}

func (s *store) reset() {
	ctx := context.Background()

	purchases := s.client.Iap.FindMany().Delete().Tx()
	err := s.client.Prisma.Transaction(purchases).Exec(ctx)
	if err != nil {
		panic(err)
	}
}

func NewInPostgres(client *db.PrismaClient) iap.Store {
	return &store{
		client,
	}
}

func (s *store) CreatePurchase(ctx context.Context, purchase *iap.Purchase) error {
	if purchase.Product != iap.ProductCreateAccount {
		return errors.New("product must be create account")
	}
	if purchase.State != iap.StateFulfilled {
		return errors.New("state must be fulfilled")
	}

	encodedReceiptID := pg.Encode(purchase.ReceiptID)
	encodedUserID := pg.Encode(purchase.User.Value)

	_, err := s.client.Iap.FindUnique(
		db.Iap.ReceiptID.Equals(encodedReceiptID),
	).Exec(ctx)
	if err == nil {
		return iap.ErrExists
	} else if !errors.Is(err, db.ErrNotFound) {
		return err
	}

	_, err = s.client.Iap.CreateOne(
		db.Iap.ReceiptID.Set(encodedReceiptID),
		db.Iap.UserID.Set(encodedUserID),
		db.Iap.Platform.Set(int(purchase.Platform)),
		db.Iap.Product.Set(int(purchase.Product)),
		db.Iap.State.Set(int(purchase.State)),
	).Exec(ctx)
	return err
}

func (s *store) GetPurchase(ctx context.Context, receiptID []byte) (*iap.Purchase, error) {
	encodedReceiptID := pg.Encode(receiptID)

	res, err := s.client.Iap.FindUnique(
		db.Iap.ReceiptID.Equals(encodedReceiptID),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) {
		return nil, iap.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	return fromModel(res)
}

func fromModel(m *db.IapModel) (*iap.Purchase, error) {
	decodedReceiptID, err := pg.Decode(m.ReceiptID)
	if err != nil {
		return nil, err
	}

	decodedUserID, err := pg.Decode(m.UserID)
	if err != nil {
		return nil, err
	}

	return &iap.Purchase{
		ReceiptID: decodedReceiptID,
		Platform:  commonpb.Platform(m.Platform),
		User:      &commonpb.UserId{Value: decodedUserID},
		Product:   iap.Product(m.Product),
		State:     iap.State(m.State),
		CreatedAt: m.CreatedAt,
	}, nil
}
