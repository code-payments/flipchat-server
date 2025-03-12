package postgres

import (
	"context"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/code-payments/flipchat-server/intent"
)

type store struct {
	pool *pgxpool.Pool
}

func NewInPostgres(pool *pgxpool.Pool) intent.Store {
	return &store{
		pool: pool,
	}
}

func (s *store) IsFulfilled(ctx context.Context, id *commonpb.IntentId) (bool, error) {
	return dbIsFulfilled(ctx, s.pool, id)
}

func (s *store) MarkFulfilled(ctx context.Context, id *commonpb.IntentId) error {
	isFulfilled, err := dbIsFulfilled(ctx, s.pool, id)
	if err != nil {
		return err
	} else if isFulfilled {
		return intent.ErrAlreadyFulfilled
	}
	return dbMarkFulfilled(ctx, s.pool, id)
}

func (s *store) reset() {
	_, err := s.pool.Exec(context.Background(), "DELETE FROM "+intentsTableName)
	if err != nil {
		panic(err)
	}
}
