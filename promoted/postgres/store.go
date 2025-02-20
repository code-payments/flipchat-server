package postgres

import (
	"context"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	pg "github.com/code-payments/flipchat-server/database/postgres"

	"github.com/code-payments/flipchat-server/database/prisma/db"
	"github.com/code-payments/flipchat-server/promoted"
)

type store struct {
	client *db.PrismaClient
}

// reset clears the PromotedChat table (used for testing).
func (s *store) reset() {
	ctx := context.Background()

	promotedChats := s.client.PromotedChat.FindMany().Delete().Tx()
	err := s.client.Prisma.Transaction(promotedChats).Exec(ctx)
	if err != nil {
		panic(err)
	}
}

// NewInPostgres creates a new PostgreSQL store for Promoted Chats.
func NewInPostgres(client *db.PrismaClient) promoted.Store {
	return &store{
		client: client,
	}
}

// GetPromotedChats retrieves promoted chats by topic from PostgreSQL.
func (s *store) GetPromotedChats(ctx context.Context, topic string) ([]*promoted.PromotedChat, error) {

	prChats, err := s.client.PromotedChat.FindMany(
		db.PromotedChat.Topic.Equals(topic),
	).OrderBy(
		db.PromotedChat.Score.Order(db.SortOrderDesc),
	).Exec(ctx)
	if err != nil {
		return nil, err
	}

	var chats []*promoted.PromotedChat
	for _, prChat := range prChats {

		decodedChatID, err := pg.Decode(prChat.ChatID)
		if err != nil {
			return nil, err
		}

		chats = append(chats, &promoted.PromotedChat{
			ChatID:    &commonpb.ChatId{Value: decodedChatID},
			Score:     prChat.Score,
			Topic:     prChat.Topic,
			CreatedAt: prChat.CreatedAt,
			UpdatedAt: prChat.UpdatedAt,
		})
	}

	return chats, nil
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

	encodedChatID := pg.Encode(chatID.Value)

	_, err := s.client.PromotedChat.UpsertOne(
		db.PromotedChat.ChatIDTopic(
			db.PromotedChat.ChatID.Equals(encodedChatID),
			db.PromotedChat.Topic.Equals(topic),
		),
	).Create(
		db.PromotedChat.Chat.Link(db.Chat.ID.Equals(encodedChatID)),
		db.PromotedChat.Topic.Set(topic),
		db.PromotedChat.Score.Set(score),
	).Update(
		db.PromotedChat.Score.Set(score),
	).Exec(ctx)

	return err
}

func (s *store) DemoteChat(ctx context.Context, chatID *commonpb.ChatId, topic string) error {
	if chatID == nil {
		return promoted.ErrInvalidChatID
	}

	if topic == "" {
		return promoted.ErrInvalidTopic
	}

	encodedChatID := pg.Encode(chatID.Value)

	_, err := s.client.PromotedChat.FindMany(
		db.PromotedChat.ChatID.Equals(encodedChatID),
		db.PromotedChat.Topic.Equals(topic),
	).Delete().Exec(ctx)

	if err != nil {
		return promoted.ErrNotFound
	}

	return nil
}
