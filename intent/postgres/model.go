package postgres

import (
	"context"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5/pgxpool"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	pg "github.com/code-payments/flipchat-server/database/postgres"
)

const (
	intentsTableName = `flipchat_intents`
	allIntentFields  = `"id", "isFulfilled", "createdAt", "updatedAt"`
)

func dbIsFulfilled(ctx context.Context, pool *pgxpool.Pool, id *commonpb.IntentId) (bool, error) {
	var res bool
	query := `SELECT "isFulfilled" FROM ` + intentsTableName + ` WHERE "id" = $1`
	err := pgxscan.Get(
		ctx,
		pool,
		&res,
		query,
		pg.Encode(id.Value),
	)
	if pgxscan.NotFound(err) {
		return false, nil
	}
	return res, err
}

func dbMarkFulfilled(ctx context.Context, pool *pgxpool.Pool, id *commonpb.IntentId) error {
	query := `INSERT INTO ` + intentsTableName + ` (` + allIntentFields + `) VALUES ($1, true, NOW(), NOW()) ON CONFLICT ("id") DO NOTHING`
	_, err := pool.Exec(ctx, query, pg.Encode(id.Value))
	return err
}
