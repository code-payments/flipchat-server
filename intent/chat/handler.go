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
