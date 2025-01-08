package messaging

import (
	"bytes"
	"context"
	"errors"

	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
	codecurrency "github.com/code-payments/code-server/pkg/currency"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/intent"
)

type SendTipMessagePaymentIntentHandler struct {
	accounts account.Store
	messages MessageStore
}

func NewSendTipMessagePaymentIntentHandler(accounts account.Store, messages MessageStore) *SendTipMessagePaymentIntentHandler {
	return &SendTipMessagePaymentIntentHandler{
		accounts: accounts,
		messages: messages,
	}
}

func (h *SendTipMessagePaymentIntentHandler) Validate(ctx context.Context, intentRecord *codeintent.Record, customMetadata proto.Message) (*intent.ValidationResult, error) {
	sendTipMessageMetadata, ok := customMetadata.(*messagingpb.SendTipMessagePaymentMetadata)
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
	if !bytes.Equal(sendTipMessageMetadata.TipperId.Value, payingUser.Value) {
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

	message, err := h.messages.GetMessage(ctx, sendTipMessageMetadata.ChatId, sendTipMessageMetadata.MessageId)
	if err == ErrMessageNotFound {
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
