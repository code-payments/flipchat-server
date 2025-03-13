package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/promoted"
)

type store struct {
	pool *pgxpool.Pool
}

// NewInPostgres creates a new PostgreSQL store for Promoted Chats.
func NewInPostgres(pool *pgxpool.Pool) promoted.Store {
	return &store{
		pool: pool,
	}
}

// GetPromotedChats retrieves promoted chats by topic from PostgreSQL.
func (s *store) GetPromotedChats(ctx context.Context, topic string) ([]*promoted.PromotedChat, error) {
	models, err := dbGetPromotedChats(ctx, s.pool, topic)
	if err != nil {
		return nil, err
	}

	res := make([]*promoted.PromotedChat, len(models))
	for i, model := range models {
		res[i], err = fromModel(model)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

// PromoteChat promotes a chat (or updates the score if it already exists).
func (s *store) PromoteChat(ctx context.Context, chatID *commonpb.ChatId, topic string, score int) error {
	if chatID == nil {
		return promoted.ErrInvalidChatID
	}

	if score < 0 {
		return promoted.ErrInvalidScore
	}

	if topic == "" {
		return promoted.ErrInvalidTopic
	}

	model, err := toModel(&promoted.PromotedChat{
		ChatID: chatID,
		Topic:  topic,
		Score:  score,
	})
	if err != nil {
		return err
	}

	return model.dbUpsert(ctx, s.pool)
}

func (s *store) DemoteChat(ctx context.Context, chatID *commonpb.ChatId, topic string) error {
	if chatID == nil {
		return promoted.ErrInvalidChatID
	}

	if topic == "" {
		return promoted.ErrInvalidTopic
	}

	return dbDemoteChat(ctx, s.pool, chatID, topic)
}

func (s *store) reset() {
	_, err := s.pool.Exec(context.Background(), "DELETE FROM "+promotedChatsTableName)
	if err != nil {
		panic(err)
	}
}
