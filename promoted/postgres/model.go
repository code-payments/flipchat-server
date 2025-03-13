package postgres

import (
	"context"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5/pgxpool"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	pg "github.com/code-payments/flipchat-server/database/postgres"
	"github.com/code-payments/flipchat-server/promoted"
)

const (
	promotedChatsTableName = "flipchat_promotedchats"
	allPromotedChatFields  = `"chatId", "topic", "score", "createdAt", "updatedAt"`
)

type model struct {
	ChatID    string    `db:"chatId"`
	Topic     string    `db:"topic"`
	Score     int       `db:"score"`
	CreatedAt time.Time `db:"createdAt"`
	UpdatedAt time.Time `db:"updatedAt"`
}

func toModel(promotedChat *promoted.PromotedChat) (*model, error) {
	return &model{
		ChatID: pg.Encode(promotedChat.ChatID.Value),
		Topic:  promotedChat.Topic,
		Score:  promotedChat.Score,
	}, nil
}

func fromModel(m *model) (*promoted.PromotedChat, error) {
	decodedChatID, err := pg.Decode(m.ChatID)
	if err != nil {
		return nil, err
	}
	return &promoted.PromotedChat{
		ChatID: &commonpb.ChatId{Value: decodedChatID},
		Topic:  m.Topic,
		Score:  m.Score,
	}, nil
}

func (m *model) dbUpsert(ctx context.Context, pool *pgxpool.Pool) error {
	query := `INSERT INTO ` + promotedChatsTableName + ` (` + allPromotedChatFields + `) VALUES ($1, $2, $3, NOW(), NOW()) ON CONFLICT ("chatId", "topic") DO UPDATE SET "score" = $3 WHERE ` + promotedChatsTableName + `."chatId" = $1 AND ` + promotedChatsTableName + `."topic" = $2 RETURNING` + allPromotedChatFields
	return pgxscan.Get(
		ctx,
		pool,
		m,
		query,
		m.ChatID,
		m.Topic,
		m.Score,
	)
}

func dbGetPromotedChats(ctx context.Context, pool *pgxpool.Pool, topic string) ([]*model, error) {
	var res []*model
	query := `SELECT ` + allPromotedChatFields + ` FROM ` + promotedChatsTableName + ` WHERE "topic" = $1 ORDER BY "score" DESC`
	err := pgxscan.Select(
		ctx,
		pool,
		&res,
		query,
		topic,
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, promoted.ErrNotFound
		}
		return nil, err
	}
	return res, nil
}

func dbDemoteChat(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, topic string) error {
	query := `DELETE FROM ` + promotedChatsTableName + ` WHERE "chatId" = $1 AND "topic" = $2`
	_, err := pool.Exec(
		ctx,
		query,
		pg.Encode(chatID.Value),
		topic,
	)
	return err
}
