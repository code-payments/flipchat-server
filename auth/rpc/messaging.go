package database

import (
	"context"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/chat"
)

type MessagingAuthorizer struct {
	chats chat.Store
}

func NewMessagingRpcAuthorizer(chats chat.Store) *MessagingAuthorizer {
	return &MessagingAuthorizer{
		chats: chats,
	}
}

func (a *MessagingAuthorizer) CanStreamMessages(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	return a.chats.IsMember(ctx, chatID, userID)
}

func (a *MessagingAuthorizer) CanGetMessages(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	return a.chats.IsMember(ctx, chatID, userID)
}

func (a *MessagingAuthorizer) CanSendMessage(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	member, err := a.chats.GetMember(ctx, chatID, userID)
	if err == chat.ErrMemberNotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return !member.IsMuted && member.HasSendPermission, nil
}

func (a *MessagingAuthorizer) CanAdvancePointer(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	return a.chats.IsMember(ctx, chatID, userID)
}

func (a *MessagingAuthorizer) CanNotifyIsTyping(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	return a.chats.IsMember(ctx, chatID, userID)
}
