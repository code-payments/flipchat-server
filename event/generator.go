package event

import (
	"context"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

type ProfileGenerator interface {
	OnProfileUpdated(ctx context.Context, userID *commonpb.UserId)
}
