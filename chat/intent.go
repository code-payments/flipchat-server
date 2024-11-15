package chat

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	"google.golang.org/protobuf/proto"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
	codecurrency "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/intent"
)

type JoinChatPaymentIntentHandler struct {
	accounts account.Store
	chats    Store
}

func NewJoinChatPaymentIntentHandler(accounts account.Store, chats Store) *JoinChatPaymentIntentHandler {
	return &JoinChatPaymentIntentHandler{
		accounts: accounts,
		chats:    chats,
	}
}

func (h *JoinChatPaymentIntentHandler) Validate(ctx context.Context, intentRecord codeintent.Record, customMetadata proto.Message) (*intent.ValidationResult, error) {
	joinChatMetadata, ok := customMetadata.(*chatpb.JoinChatPaymentMetadata)
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

	paidOwner, err := codecommon.NewAccountFromPublicKeyString(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount)
	if err != nil {
		return nil, err
	}

	chat, err := h.chats.GetChatMetadata(ctx, joinChatMetadata.ChatId)
	if err == ErrChatNotFound {
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

	// Chat must enforce a cover charge
	if chat.CoverCharge == nil {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "chat does not have a cover charge",
		}, nil
	}

	// Payment amount must be exactly the cover charge
	if intentRecord.SendPublicPaymentMetadata.ExchangeCurrency != codecurrency.KIN || intentRecord.SendPublicPaymentMetadata.Quantity != chat.CoverCharge.Quarks {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: fmt.Sprintf("cover charge is %d quarks", chat.CoverCharge.Quarks),
		}, nil
	}

	// The paying user must pay for their own join
	if !bytes.Equal(joinChatMetadata.UserId.Value, payingUser.Value) {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "user must pay for their own join",
		}, nil
	}

	// The payment must go to the current chat owner
	if !bytes.Equal(paidUser.Value, chat.Owner.Value) {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "payment must go to chat owner",
		}, nil
	}

	// The paying user is already a member, and doesn't need to join
	isMember, err := h.chats.IsMember(ctx, chat.ChatId, payingUser)
	if err != nil {
		return nil, err
	} else if isMember {
		return &intent.ValidationResult{
			StatusCode:       intent.INVALID,
			ErrorDescription: "user is already a chat member",
		}, nil
	}

	return &intent.ValidationResult{StatusCode: intent.SUCCESS}, nil
}
