package promoted

import (
	"context"
	"errors"
	"time"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

var (
	ErrNotFound      = errors.New("promoted chat not found")
	ErrExists        = errors.New("promoted chat already exists")
	ErrInvalidChatID = errors.New("invalid chat ID")
	ErrInvalidScore  = errors.New("invalid score")
	ErrInvalidTopic  = errors.New("invalid topic")
)

// Store defines the interface for Promoted Chat storage.
type Store interface {
	// GetPromotedChats retrieves promoted chats by topic.
	GetPromotedChats(ctx context.Context, topic string) ([]*PromotedChat, error)

	// PromoteChat promotes a chat (or updates the score if it already exists).
	PromoteChat(ctx context.Context, chatID *commonpb.ChatId, topic string, score int) error

	// DemoteChat demotes a chat (remove it from the promoted list for the topic).
	DemoteChat(ctx context.Context, chatID *commonpb.ChatId, topic string) error
}

// PromotedChat represents a promoted chat entity.
type PromotedChat struct {
	ChatID    *commonpb.ChatId
	Score     int
	Topic     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Clone creates a deep copy of the PromotedChat.
func (pc *PromotedChat) Clone() *PromotedChat {
	return &PromotedChat{
		ChatID:    pc.ChatID,
		Score:     pc.Score,
		Topic:     pc.Topic,
		CreatedAt: pc.CreatedAt,
		UpdatedAt: pc.UpdatedAt,
	}
}
