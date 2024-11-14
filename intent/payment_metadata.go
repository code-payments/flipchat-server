package intent

import (
	"context"
	"errors"

	"github.com/mr-tron/base58"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	codetransactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	codedata "github.com/code-payments/code-server/pkg/code/data"
	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
)

var (
	ErrNoPaymentMetadata = errors.New("no payment metdata")
)

func LoadPaymentMetadata(ctx context.Context, codeData codedata.Provider, intentID *commonpb.IntentId, dst proto.Message) error {
	intentRecord, err := codeData.GetIntent(ctx, base58.Encode(intentID.Value))
	if err == codeintent.ErrIntentNotFound {
		return ErrNoPaymentMetadata
	} else if err != nil {
		return err
	}

	if len(intentRecord.ExtendedMetadata) == 0 {
		return ErrNoPaymentMetadata
	}

	var extendedPaymentMetdata codetransactionpb.ExtendedPaymentMetadata
	err = proto.Unmarshal(intentRecord.ExtendedMetadata, &extendedPaymentMetdata)
	if err != nil {
		return err
	}

	return anypb.UnmarshalTo(extendedPaymentMetdata.Value, dst, proto.UnmarshalOptions{})
}
