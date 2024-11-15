package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	codetransactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	codedata "github.com/code-payments/code-server/pkg/code/data"
	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
	codecurrency "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/flipchat-server/model"
)

func CreatePayment(t *testing.T, codeData codedata.Provider, kinAmount uint64, mtdt proto.Message) *commonpb.IntentId {
	intentID := model.MustGenerateIntentID()

	extendedMetadata := codetransactionpb.ExtendedPaymentMetadata{Value: &anypb.Any{}}
	require.NoError(t, anypb.MarshalFrom(extendedMetadata.Value, mtdt, proto.MarshalOptions{}))
	extendedMetadataBytes, err := proto.Marshal(&extendedMetadata)
	require.NoError(t, err)

	intentRecord := &codeintent.Record{
		IntentId:   model.IntentIDString(intentID),
		IntentType: codeintent.SendPublicPayment,
		SendPublicPaymentMetadata: &codeintent.SendPublicPaymentMetadata{
			DestinationOwnerAccount: "todo", // todo: value doesn't actually matter, it's assumed to be validated in SubmitIntent
			DestinationTokenAccount: "todo", // todo: value doesn't actually matter, it's assumed to be validated in SubmitIntent
			Quantity:                kin.ToQuarks(kinAmount),

			ExchangeCurrency: codecurrency.KIN,
			ExchangeRate:     1.0,
			NativeAmount:     float64(kinAmount),
			UsdMarketValue:   1.0,

			IsWithdrawal: true,
		},
		InitiatorOwnerAccount: "todo", // todo: value doesn't actually matter, it's assumed to be validated in SubmitIntent
		ExtendedMetadata:      extendedMetadataBytes,
		State:                 codeintent.StateConfirmed,
	}

	require.NoError(t, codeData.SaveIntent(context.Background(), intentRecord))

	return intentID
}
