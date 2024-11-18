package intent

import (
	"context"
	"errors"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

var (
	ErrAlreadyFulfilled = errors.New("intent id is already fulfilled")
)

type Store interface {
	IsFulfilled(ctx context.Context, id *commonpb.IntentId) (bool, error)

	MarkFulfilled(ctx context.Context, id *commonpb.IntentId) error
}
