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
	intents  intent.Store
	messages messaging.MessageStore
	codeData codedata.Provider
}

func NewMessagingRpcAuthorizer(chats chat.Store, intents intent.Store, messages messaging.MessageStore, codeData codedata.Provider) *MessagingAuthorizer {
	return &MessagingAuthorizer{
		chats:    chats,
		intents:  intents,
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

// todo: This needs a refactor/cleanup because it's blowing up in size/complexity
func (a *MessagingAuthorizer) CanSendMessage(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, content *messagingpb.Content, paymentIntent *commonpb.IntentId) (bool, string, error) {
	chatMd, err := a.chats.GetChatMetadata(ctx, chatID)
	if err == chat.ErrChatNotFound {
		return false, "chat not found", nil
	} else if err != nil {
		return false, "", err
	}

	isOwner := chatMd.Owner != nil && bytes.Equal(chatMd.Owner.Value, userID.Value)

	// todo: individual handlers for different content types
	var canSendWhenClosed bool
	var requiresListenerPayment bool
	switch typed := content.Type.(type) {
	case *messagingpb.Content_Text:
		requiresListenerPayment = true
	case *messagingpb.Content_Reaction:
		canSendWhenClosed = true

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
		requiresListenerPayment = true

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
		canSendWhenClosed = true

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

		isFulfilled, err := a.intents.IsFulfilled(ctx, paymentIntent)
		if err != nil {
			return false, "", err
		} else if isFulfilled {
			return false, "intent already fulifilled", nil
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
		canSendWhenClosed = true

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
		if isOwner {
			hasDeletePermission = true
		}
		if referenceMessage.SenderId != nil && bytes.Equal(referenceMessage.SenderId.Value, userID.Value) {
			hasDeletePermission = true
		}
		if !hasDeletePermission {
			return false, "chat member doesn't have delete permission", nil
		}
	case *messagingpb.Content_Review:
		canSendWhenClosed = true

		referenceMessage, err := a.messages.GetMessage(ctx, chatID, typed.Review.OriginalMessageId)
		if err == messaging.ErrMessageNotFound {
			return false, "reference not found", nil
		} else if err != nil {
			return false, "", err
		}

		if !referenceMessage.WasSenderOffStage {
			return false, "reference message must have been sent off stage", nil
		}

		switch referenceMessage.Content[0].Type.(type) {
		case *messagingpb.Content_Text, *messagingpb.Content_Reply:
		default:
			return false, "invalid reference content type", nil
		}

		// todo: this will be expanded to other members
		if !isOwner {
			return false, "only chat owners can review messages", nil
		}
	default:
		return false, "unsupported content type", nil
	}

	if !canSendWhenClosed && !isOwner && chatMd.OpenStatus != nil && !chatMd.OpenStatus.IsCurrentlyOpen {
		return false, "chat is closed", nil
	}

	member, err := a.chats.GetMember(ctx, chatID, userID)
	if err == chat.ErrMemberNotFound {
		return false, "not a chat member", nil
	} else if err != nil {
		return false, "", err
	}

	if member.IsMuted {
		return false, "chat member is muted", nil
	}

	if !isOwner && !member.HasSendPermission && requiresListenerPayment {
		if paymentIntent == nil {
			return false, "payment not provided", nil
		}

		var paymentMetadata messagingpb.SendMessageAsListenerPaymentMetadata
		_, err := intent.LoadPaymentMetadata(ctx, a.codeData, paymentIntent, &paymentMetadata)
		if err == intent.ErrNoPaymentMetadata {
			return false, "payment metadata missing", nil
		} else if err == intent.ErrInvalidPaymentMetadata {
			return false, "invalid payment metadata", nil
		} else if err != nil {
			return false, "", err
		}

		isFulfilled, err := a.intents.IsFulfilled(ctx, paymentIntent)
		if err != nil {
			return false, "", err
		} else if isFulfilled {
			return false, "intent already fulifilled", nil
		}

		if !bytes.Equal(chatID.Value, paymentMetadata.ChatId.Value) {
			return false, "payment references a different chat", nil
		}
		if !bytes.Equal(userID.Value, paymentMetadata.UserId.Value) {
			return false, "payment references a different user", nil
		}
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
