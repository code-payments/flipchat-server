package messaging

import (
	"context"
	"errors"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	"github.com/code-payments/flipchat-server/query"
)

var (
	ErrMessageNotFound = errors.New("message not found")
)

type UserPointer struct {
	UserID  *commonpb.UserId
	Pointer *messagingpb.Pointer
}

type MessageStore interface {
	GetMessage(ctx context.Context, chatID *commonpb.ChatId, messageID *messagingpb.MessageId) (*messagingpb.Message, error)
	GetMessages(ctx context.Context, chatID *commonpb.ChatId, options ...query.Option) ([]*messagingpb.Message, error)
	PutMessage(ctx context.Context, chatID *commonpb.ChatId, msg *messagingpb.Message) (*messagingpb.Message, error)
	PutMessageLegacy(ctx context.Context, chatID *commonpb.ChatId, msg *messagingpb.Message) (*messagingpb.Message, error)
	CountUnread(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, lastRead *messagingpb.MessageId, maxValue int64) (int64, error)
}

type PointerStore interface {
	AdvancePointer(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, pointer *messagingpb.Pointer) (bool, error)
	GetPointers(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) ([]*messagingpb.Pointer, error)
	GetAllPointers(ctx context.Context, chatID *commonpb.ChatId) ([]UserPointer, error)
}
