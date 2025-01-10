package auth

import (
	"context"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
)

type Messaging interface {
	CanStreamMessages(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, string, error)

	CanGetMessage(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, string, error)

	CanGetMessages(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, string, error)

	CanSendMessage(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, content *messagingpb.Content, paymentIntent *commonpb.IntentId) (bool, string, error)

	CanAdvancePointer(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, string, error)

	CanNotifyIsTyping(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, string, error)
}
