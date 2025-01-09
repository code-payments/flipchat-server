package database

import (
	"bytes"
	"context"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	codedata "github.com/code-payments/code-server/pkg/code/data"

	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/intent"
	"github.com/code-payments/flipchat-server/messaging"
)

// todo: this needs tests all around
type MessagingAuthorizer struct {
	chats    chat.Store
	messages messaging.MessageStore
	codeData codedata.Provider
}

func NewMessagingRpcAuthorizer(chats chat.Store, messages messaging.MessageStore, codeData codedata.Provider) *MessagingAuthorizer {
	return &MessagingAuthorizer{
		chats:    chats,
		messages: messages,
		codeData: codeData,
	}
}

func (a *MessagingAuthorizer) CanStreamMessages(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	return a.chats.IsMember(ctx, chatID, userID)
}

func (a *MessagingAuthorizer) CanGetMessage(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	return a.chats.IsMember(ctx, chatID, userID)
}

func (a *MessagingAuthorizer) CanGetMessages(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	return a.chats.IsMember(ctx, chatID, userID)
}

func (a *MessagingAuthorizer) CanSendMessage(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, content *messagingpb.Content, paymentIntent *commonpb.IntentId) (bool, error) {
	// todo: individual handlers for different content types
	switch typed := content.Type.(type) {
	case *messagingpb.Content_Text:
	case *messagingpb.Content_Reaction:
		referenceMessage, err := a.messages.GetMessage(ctx, chatID, typed.Reaction.OriginalMessageId)
		if err == messaging.ErrMessageNotFound {
			return false, nil
		} else if err != nil {
			return false, err
		}

		switch referenceMessage.Content[0].Type.(type) {
		case *messagingpb.Content_Text, *messagingpb.Content_Reply:
		default:
			return false, nil
		}
	case *messagingpb.Content_Reply:
		referenceMessage, err := a.messages.GetMessage(ctx, chatID, typed.Reply.OriginalMessageId)
		if err == messaging.ErrMessageNotFound {
			return false, nil
		} else if err != nil {
			return false, err
		}

		switch referenceMessage.Content[0].Type.(type) {
		case *messagingpb.Content_Text, *messagingpb.Content_Reply:
		default:
			return false, nil
		}
	case *messagingpb.Content_Tip:
		referenceMessage, err := a.messages.GetMessage(ctx, chatID, typed.Tip.OriginalMessageId)
		if err == messaging.ErrMessageNotFound {
			return false, nil
		} else if err != nil {
			return false, err
		}

		switch referenceMessage.Content[0].Type.(type) {
		case *messagingpb.Content_Text, *messagingpb.Content_Reply:
		default:
			return false, nil
		}

		if paymentIntent == nil {
			return false, nil
		}

		var paymentMetadata messagingpb.SendTipMessagePaymentMetadata
		intentRecord, err := intent.LoadPaymentMetadata(ctx, a.codeData, paymentIntent, &paymentMetadata)
		if err == intent.ErrNoPaymentMetadata {
			return false, nil
		} else if err != nil {
			return false, err
		}

		if !bytes.Equal(chatID.Value, paymentMetadata.ChatId.Value) {
			return false, nil
		}
		if !bytes.Equal(typed.Tip.OriginalMessageId.Value, paymentMetadata.MessageId.Value) {
			return false, nil
		}
		if !bytes.Equal(userID.Value, paymentMetadata.TipperId.Value) {
			return false, nil
		}
		if intentRecord.SendPublicPaymentMetadata.Quantity != typed.Tip.TipAmount.Quarks {
			return false, nil
		}
	case *messagingpb.Content_Deleted:
		chatMd, err := a.chats.GetChatMetadata(ctx, chatID)
		if err == chat.ErrChatNotFound {
			return false, nil
		} else if err != nil {
			return false, err
		}

		referenceMessage, err := a.messages.GetMessage(ctx, chatID, typed.Deleted.OriginalMessageId)
		if err == messaging.ErrMessageNotFound {
			return false, nil
		} else if err != nil {
			return false, err
		}

		switch referenceMessage.Content[0].Type.(type) {
		case *messagingpb.Content_Text, *messagingpb.Content_Reply, *messagingpb.Content_Reaction:
		default:
			return false, nil
		}

		if !bytes.Equal(chatMd.Owner.Value, userID.Value) && !bytes.Equal(referenceMessage.SenderId.Value, userID.Value) {
			return false, nil
		}
	default:
		return false, nil
	}

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
