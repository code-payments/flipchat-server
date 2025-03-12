package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5/pgxpool"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	pg "github.com/code-payments/flipchat-server/database/postgres"
	"github.com/code-payments/flipchat-server/iap"
)

const (
	iapsTableName = "flipchat_iap"
	allIapFields  = `"receiptId", "platform", "userId", "product", "state", "createdAt"`
)

type model struct {
	ReceiptID string    `db:"receiptId"`
	Platform  int       `db:"platform"`
	UserID    string    `db:"userId"`
	Product   int       `db:"product"`
	State     int       `db:"state"`
	CreatedAt time.Time `db:"createdAt"`
}

func toModel(purchase *iap.Purchase) (*model, error) {
	return &model{
		ReceiptID: pg.Encode(purchase.ReceiptID),
		Platform:  int(purchase.Platform),
		UserID:    pg.Encode(purchase.User.Value),
		Product:   int(purchase.Product),
		State:     int(purchase.State),
	}, nil
}

func fromModel(m *model) (*iap.Purchase, error) {
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
	}, nil
}

func (m *model) dbPut(ctx context.Context, pool *pgxpool.Pool) error {
	query := `INSERT INTO ` + iapsTableName + `(` + allIapFields + `) VALUES ($1, $2, $3, $4, $5, NOW()) RETURNING ` + allIapFields
	err := pgxscan.Get(
		ctx,
		pool,
		m,
		query,
		m.ReceiptID,
		m.Platform,
		m.UserID,
		m.Product,
		m.State,
	)
	if err == nil {
		return nil
	} else if strings.Contains(err.Error(), "23505") { // todo: better utility for detecting unique violations
		return iap.ErrExists
	}
	return err
}

func dbGetPurchase(ctx context.Context, pool *pgxpool.Pool, receiptID []byte) (*model, error) {
	res := &model{}
	query := `SELECT ` + allIapFields + ` FROM ` + iapsTableName + ` WHERE "receiptId" = $1`
	err := pgxscan.Get(
		ctx,
		pool,
		res,
		query,
		pg.Encode(receiptID),
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, iap.ErrNotFound
		}
		return nil, err
	}
	return res, nil
}
