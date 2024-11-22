package auth

import (
	"context"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

type Messaging interface {
	CanStreamMessages(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error)

	CanGetMessages(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error)

	CanSendMessage(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error)

	CanAdvancePointer(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error)

	CanNotifyIsTyping(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error)
}
