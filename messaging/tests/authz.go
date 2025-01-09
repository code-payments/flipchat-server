package tests

import (
	"context"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
)

type AlwaysAllowRpcAuthz struct {
}

func NewAlwaysAllowRpcAuthz() *AlwaysAllowRpcAuthz {
	return &AlwaysAllowRpcAuthz{}
}

func (a *AlwaysAllowRpcAuthz) CanStreamMessages(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	return true, nil
}

func (a *AlwaysAllowRpcAuthz) CanGetMessage(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	return true, nil
}

func (a *AlwaysAllowRpcAuthz) CanGetMessages(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	return true, nil
}

func (a *AlwaysAllowRpcAuthz) CanSendMessage(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, content *messagingpb.Content, paymentIntent *commonpb.IntentId) (bool, error) {
	return true, nil
}

func (a *AlwaysAllowRpcAuthz) CanAdvancePointer(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	return true, nil
}

func (a *AlwaysAllowRpcAuthz) CanNotifyIsTyping(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	return true, nil
}
