package chat

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
	codecurrency "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/flags"
	"github.com/code-payments/flipchat-server/intent"
)

type StartGroupChatPaymentIntentHandler struct {
	accounts account.Store
	chats    chat.Store
}

func NewStartGroupChatPaymentIntentHandler(accounts account.Store, chats chat.Store) *StartGroupChatPaymentIntentHandler {
	return &StartGroupChatPaymentIntentHandler{
		accounts: accounts,
		chats:    chats,
	}
}

func (h *StartGroupChatPaymentIntentHandler) Validate(ctx context.Context, intentRecord *codeintent.Record, customMetadata proto.Message) (*intent.ValidationResult, error) {
	typedMetadata, ok := customMetadata.(*chatpb.StartGroupChatPaymentMetadata)
	if !ok {
		return nil, errors.New("unexepected custom metadata")
	}

	payingOwner, err := codecommon.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
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

	// Payment must be public
	if intentRecord.IntentType != codeintent.SendPublicPayment {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "payment must be public",
		}, nil
	}

	// Payment amount must be exactly the group creation fee
	if intentRecord.SendPublicPaymentMetadata.ExchangeCurrency != codecurrency.KIN || intentRecord.SendPublicPaymentMetadata.Quantity != flags.StartGroupFee {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: fmt.Sprintf("fee payment must be exactly %d quarks", flags.StartGroupFee),
		}, nil
	}

	// Payment must go to the fee destination
	if intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount != flags.FeeDestination.PublicKey().ToBase58() {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: fmt.Sprintf("fee destination must be %s", flags.FeeDestination.PublicKey().ToBase58()),
		}, nil
	}

	// The paying user must pay for their own group creation
	if !bytes.Equal(typedMetadata.UserId.Value, payingUser.Value) {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "user must pay for their own group creation",
		}, nil
	}

	return &intent.ValidationResult{StatusCode: intent.SUCCESS}, nil
}

type JoinChatPaymentIntentHandler struct {
	accounts account.Store
	chats    chat.Store
}

func NewJoinChatPaymentIntentHandler(accounts account.Store, chats chat.Store) *JoinChatPaymentIntentHandler {
	return &JoinChatPaymentIntentHandler{
		accounts: accounts,
		chats:    chats,
	}
}

func (h *JoinChatPaymentIntentHandler) Validate(ctx context.Context, intentRecord *codeintent.Record, customMetadata proto.Message) (*intent.ValidationResult, error) {
	typedMetadata, ok := customMetadata.(*chatpb.JoinChatPaymentMetadata)
	if !ok {
		return nil, errors.New("unexepected custom metadata")
	}

	payingOwner, err := codecommon.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
	if err != nil {
		return nil, err
	}

	paidOwner, err := codecommon.NewAccountFromPublicKeyString(intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount)
	if err != nil {
		return nil, err
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

	// Chat must enforce a cover charge
	if chatMd.CoverCharge == nil {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "chat does not have a cover charge",
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
	if intentRecord.SendPublicPaymentMetadata.ExchangeCurrency != codecurrency.KIN || intentRecord.SendPublicPaymentMetadata.Quantity != chatMd.CoverCharge.Quarks {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: fmt.Sprintf("cover charge is %d quarks", chatMd.CoverCharge.Quarks),
		}, nil
	}

	// The paying user must pay for their own join
	if !bytes.Equal(typedMetadata.UserId.Value, payingUser.Value) {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "user must pay for their own join",
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
			ErrorDescription: "chat owner cannot pay to join their own chat",
		}, nil
	}

	isMember, err := h.chats.IsMember(ctx, typedMetadata.ChatId, payingUser)
	if err != nil {
		return nil, err
	}

	var hasSendPermission bool
	if isMember {
		hasSendPermission, err = h.chats.HasSendPermission(ctx, typedMetadata.ChatId, payingUser)
		if err != nil {
			return nil, err
		}
	}

	if isMember && hasSendPermission {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "user is already a chat member with send permission",
		}, nil
	}

	return &intent.ValidationResult{StatusCode: intent.SUCCESS}, nil
}
