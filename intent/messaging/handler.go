package messaging

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
	codecurrency "github.com/code-payments/code-server/pkg/currency"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/intent"
	"github.com/code-payments/flipchat-server/messaging"
)

type SendMessageAsListenerIntentHandler struct {
	accounts account.Store
	chats    chat.Store
}

func NewSendMessageAsListenerIntentHandler(accounts account.Store, chats chat.Store) *SendMessageAsListenerIntentHandler {
	return &SendMessageAsListenerIntentHandler{
		accounts: accounts,
		chats:    chats,
	}
}

func (h *SendMessageAsListenerIntentHandler) Validate(ctx context.Context, intentRecord *codeintent.Record, customMetadata proto.Message) (*intent.ValidationResult, error) {
	typedMetadata, ok := customMetadata.(*messagingpb.SendMessageAsListenerPaymentMetadata)
	if !ok {
		return nil, errors.New("unexepected custom metadata")
	}

	chatMd, err := h.chats.GetChatMetadata(ctx, typedMetadata.ChatId)
	if err == chat.ErrChatNotFound {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "chat not found",
		}, nil
	} else if err != nil {
		return nil, err
	}

	payingOwner, err := codecommon.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
	if err != nil {
		return nil, err
	}

	paidOwner, err := codecommon.NewAccountFromPublicKeyString(intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount)
	if err != nil {
		return nil, err
	}

	payingUser, err := h.accounts.GetUserId(ctx, &commonpb.PublicKey{Value: payingOwner.PublicKey().ToBytes()})
	if err == account.ErrNotFound {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "paying user not found",
		}, nil
	} else if err != nil {
		return nil, err
	}

	paidUser, err := h.accounts.GetUserId(ctx, &commonpb.PublicKey{Value: paidOwner.PublicKey().ToBytes()})
	if err == account.ErrNotFound {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "paid user not found",
		}, nil
	} else if err != nil {
		return nil, err
	}

	// Only group chats are allowed
	if chatMd.Type != chatpb.Metadata_GROUP {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "chat type must be group",
		}, nil
	}

	// Chat must enforce a messaging fee
	if chatMd.MessagingFee == nil {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "chat does not have a messaging fee",
		}, nil
	}

	// Payment must be public
	if intentRecord.IntentType != codeintent.SendPublicPayment {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "payment must be public",
		}, nil
	}

	// Payment amount must be exactly the cover charge
	//
	// todo: Should we reuse the cover charge?
	if intentRecord.SendPublicPaymentMetadata.ExchangeCurrency != codecurrency.KIN || intentRecord.SendPublicPaymentMetadata.Quantity != chatMd.MessagingFee.Quarks {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: fmt.Sprintf("messaging fee is %d quarks", chatMd.MessagingFee.Quarks),
		}, nil
	}

	// The paying user must pay for their mesage
	if !bytes.Equal(typedMetadata.UserId.Value, payingUser.Value) {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "user must pay for their message",
		}, nil
	}

	// The payment must go to the current chat owner
	if chatMd.Owner == nil {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "chat doesn't have an owner",
		}, nil
	} else if !bytes.Equal(paidUser.Value, chatMd.Owner.Value) {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "payment must go to chat owner",
		}, nil
	} else if bytes.Equal(payingUser.Value, chatMd.Owner.Value) {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "chat owner cannot pay to message",
		}, nil
	}

	return &intent.ValidationResult{StatusCode: intent.SUCCESS}, nil
}

type SendTipMessagePaymentIntentHandler struct {
	accounts account.Store
	messages messaging.MessageStore
}

func NewSendTipMessagePaymentIntentHandler(accounts account.Store, messages messaging.MessageStore) *SendTipMessagePaymentIntentHandler {
	return &SendTipMessagePaymentIntentHandler{
		accounts: accounts,
		messages: messages,
	}
}

func (h *SendTipMessagePaymentIntentHandler) Validate(ctx context.Context, intentRecord *codeintent.Record, customMetadata proto.Message) (*intent.ValidationResult, error) {
	typedMetadata, ok := customMetadata.(*messagingpb.SendTipMessagePaymentMetadata)
	if !ok {
		return nil, errors.New("unexepected custom metadata")
	}

	// Payment must be public
	if intentRecord.IntentType != codeintent.SendPublicPayment {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "payment must be public",
		}, nil
	}

	payingOwner, err := codecommon.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
	if err != nil {
		return nil, err
	}

	paidOwner, err := codecommon.NewAccountFromPublicKeyString(intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount)
	if err != nil {
		return nil, err
	}

	payingUser, err := h.accounts.GetUserId(ctx, &commonpb.PublicKey{Value: payingOwner.PublicKey().ToBytes()})
	if err == account.ErrNotFound {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "paying user not found",
		}, nil
	} else if err != nil {
		return nil, err
	}

	// The paying user must pay for their tip
	if !bytes.Equal(typedMetadata.TipperId.Value, payingUser.Value) {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "user must pay for their tip",
		}, nil
	}

	paidUser, err := h.accounts.GetUserId(ctx, &commonpb.PublicKey{Value: paidOwner.PublicKey().ToBytes()})
	if err == account.ErrNotFound {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "paid user not found",
		}, nil
	} else if err != nil {
		return nil, err
	}

	// A user cannot tip their own message
	if bytes.Equal(payingUser.Value, paidUser.Value) {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "cannot tip your own message",
		}, nil
	}

	message, err := h.messages.GetMessage(ctx, typedMetadata.ChatId, typedMetadata.MessageId)
	if err == messaging.ErrMessageNotFound {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "message not found",
		}, nil
	} else if err != nil {
		return nil, err
	}

	// Only user-generated message types can be tipped
	switch message.Content[0].Type.(type) {
	case *messagingpb.Content_Text:
	case *messagingpb.Content_Reply:
	default:
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "message type cannot be tipped",
		}, nil
	}

	// Payment must go to the message sender
	if message.SenderId == nil || !bytes.Equal(message.SenderId.Value, paidUser.Value) {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "payment must go to the message sender",
		}, nil
	}

	// Payment must be an amount denominated in Kin
	if intentRecord.SendPublicPaymentMetadata.ExchangeCurrency != codecurrency.KIN {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "payment must be made in kin",
		}, nil
	}

	return &intent.ValidationResult{StatusCode: intent.SUCCESS}, nil
}
