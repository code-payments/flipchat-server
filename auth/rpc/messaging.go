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

// todo: this needs more extensive testing
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

func (a *MessagingAuthorizer) CanStreamMessages(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, string, error) {
	return a.chatMembershipCheck(ctx, chatID, userID)
}

func (a *MessagingAuthorizer) CanGetMessage(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, string, error) {
	return a.chatMembershipCheck(ctx, chatID, userID)
}

func (a *MessagingAuthorizer) CanGetMessages(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, string, error) {
	return a.chatMembershipCheck(ctx, chatID, userID)
}

func (a *MessagingAuthorizer) CanSendMessage(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, content *messagingpb.Content, paymentIntent *commonpb.IntentId) (bool, string, error) {
	// todo: individual handlers for different content types
	switch typed := content.Type.(type) {
	case *messagingpb.Content_Text:
	case *messagingpb.Content_Reaction:
		referenceMessage, err := a.messages.GetMessage(ctx, chatID, typed.Reaction.OriginalMessageId)
		if err == messaging.ErrMessageNotFound {
			return false, "reference not found", nil
		} else if err != nil {
			return false, "", err
		}

		switch referenceMessage.Content[0].Type.(type) {
		case *messagingpb.Content_Text, *messagingpb.Content_Reply:
		default:
			return false, "invalid reference content type", nil
		}
	case *messagingpb.Content_Reply:
		referenceMessage, err := a.messages.GetMessage(ctx, chatID, typed.Reply.OriginalMessageId)
		if err == messaging.ErrMessageNotFound {
			return false, "reference not found", nil
		} else if err != nil {
			return false, "", err
		}

		switch referenceMessage.Content[0].Type.(type) {
		case *messagingpb.Content_Text, *messagingpb.Content_Reply:
		default:
			return false, "invalid reference content type", nil
		}
	case *messagingpb.Content_Tip:
		referenceMessage, err := a.messages.GetMessage(ctx, chatID, typed.Tip.OriginalMessageId)
		if err == messaging.ErrMessageNotFound {
			return false, "reference not found", nil
		} else if err != nil {
			return false, "", err
		}

		switch referenceMessage.Content[0].Type.(type) {
		case *messagingpb.Content_Text, *messagingpb.Content_Reply:
		default:
			return false, "invalid reference content type", nil
		}

		if paymentIntent == nil {
			return false, "payment not provided", nil
		}

		var paymentMetadata messagingpb.SendTipMessagePaymentMetadata
		intentRecord, err := intent.LoadPaymentMetadata(ctx, a.codeData, paymentIntent, &paymentMetadata)
		if err == intent.ErrNoPaymentMetadata {
			return false, "payment metadata missing", nil
		} else if err == intent.ErrInvalidPaymentMetadata {
			return false, "invalid payment metadata", nil
		} else if err != nil {
			return false, "", err
		}

		if !bytes.Equal(chatID.Value, paymentMetadata.ChatId.Value) {
			return false, "payment references a different chat", nil
		}
		if !bytes.Equal(typed.Tip.OriginalMessageId.Value, paymentMetadata.MessageId.Value) {
			return false, "payment references a different reference message", nil
		}
		if !bytes.Equal(userID.Value, paymentMetadata.TipperId.Value) {
			return false, "payment references a different tipper", nil
		}
		if intentRecord.SendPublicPaymentMetadata.Quantity != typed.Tip.TipAmount.Quarks {
			return false, "payment references a different amount", nil
		}
	case *messagingpb.Content_Deleted:
		chatMd, err := a.chats.GetChatMetadata(ctx, chatID)
		if err == chat.ErrChatNotFound {
			return false, "chat not found", nil
		} else if err != nil {
			return false, "", err
		}

		referenceMessage, err := a.messages.GetMessage(ctx, chatID, typed.Deleted.OriginalMessageId)
		if err == messaging.ErrMessageNotFound {
			return false, "reference not found", nil
		} else if err != nil {
			return false, "", err
		}

		switch referenceMessage.Content[0].Type.(type) {
		case *messagingpb.Content_Text, *messagingpb.Content_Reply, *messagingpb.Content_Reaction:
		default:
			return false, "invalid reference content type", nil
		}

		var hasDeletePermission bool
		if chatMd.Owner != nil && bytes.Equal(chatMd.Owner.Value, userID.Value) {
			hasDeletePermission = true
		}
		if referenceMessage.SenderId != nil && bytes.Equal(referenceMessage.SenderId.Value, userID.Value) {
			hasDeletePermission = true
		}
		if !hasDeletePermission {
			return false, "chat member doesn't have delete permission", nil
		}
	default:
		return false, "unsupported content type", nil
	}

	member, err := a.chats.GetMember(ctx, chatID, userID)
	if err == chat.ErrMemberNotFound {
		return false, "not a chat member", nil
	} else if err != nil {
		return false, "", err
	}
	if !member.HasSendPermission {
		return false, "chat member doesn't have send permission", nil
	}
	if member.IsMuted {
		return false, "chat member is muted", nil
	}
	return true, "", nil
}

func (a *MessagingAuthorizer) CanAdvancePointer(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, string, error) {
	return a.chatMembershipCheck(ctx, chatID, userID)
}

func (a *MessagingAuthorizer) CanNotifyIsTyping(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, string, error) {
	return a.chatMembershipCheck(ctx, chatID, userID)
}

func (a *MessagingAuthorizer) chatMembershipCheck(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, string, error) {
	isMember, err := a.chats.IsMember(ctx, chatID, userID)
	if err != nil {
		return false, "", err
	}
	if !isMember {
		return false, "not a chat member", nil
	}
	return true, "", nil
}
