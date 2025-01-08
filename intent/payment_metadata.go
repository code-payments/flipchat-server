package intent

import (
	"context"
	"errors"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	codetransactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	codedata "github.com/code-payments/code-server/pkg/code/data"
	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/flipchat-server/model"
)

var (
	ErrNoPaymentMetadata = errors.New("no payment metdata")
)

func LoadPaymentMetadata(ctx context.Context, codeData codedata.Provider, intentID *commonpb.IntentId, dst proto.Message) (*codeintent.Record, error) {
	intentRecord, err := codeData.GetIntent(ctx, model.IntentIDString(intentID))
	if err == codeintent.ErrIntentNotFound {
		return nil, ErrNoPaymentMetadata
	} else if err != nil {
		return nil, err
	}

	if len(intentRecord.ExtendedMetadata) == 0 {
		return nil, ErrNoPaymentMetadata
	}

	var extendedPaymentMetdata codetransactionpb.ExtendedPaymentMetadata
	err = proto.Unmarshal(intentRecord.ExtendedMetadata, &extendedPaymentMetdata)
	if err != nil {
		return nil, err
	}

	err = anypb.UnmarshalTo(extendedPaymentMetdata.Value, dst, proto.UnmarshalOptions{})
	if err != nil {
		return nil, err
	}
	return intentRecord, nil
}
