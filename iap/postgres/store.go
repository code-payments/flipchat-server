package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/code-payments/flipchat-server/iap"
)

type store struct {
	pool *pgxpool.Pool
}

func NewInPostgres(pool *pgxpool.Pool) iap.Store {
	return &store{
		pool: pool,
	}
}

func (s *store) CreatePurchase(ctx context.Context, purchase *iap.Purchase) error {
	if purchase.Product != iap.ProductCreateAccount {
		return errors.New("product must be create account")
	}
	if purchase.State != iap.StateFulfilled {
		return errors.New("state must be fulfilled")
	}

	model, err := toModel(purchase)
	if err != nil {
		return err
	}
	return model.dbPut(ctx, s.pool)
}

func (s *store) GetPurchase(ctx context.Context, receiptID []byte) (*iap.Purchase, error) {
	model, err := dbGetPurchase(ctx, s.pool, receiptID)
	if err != nil {
		return nil, err
	}
	return fromModel(model)
}

func (s *store) reset() {
	_, err := s.pool.Exec(context.Background(), "DELETE FROM "+iapsTableName)
	if err != nil {
		panic(err)
	}
}
