package messaging

import (
	"context"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
)

type Messenger interface {
	Send(ctx context.Context, chatID *commonpb.ChatId, msg *messagingpb.Message) error
}

type NoopMessenger struct {
}

func NewNoopMessenger() *NoopMessenger {
	return &NoopMessenger{}
}

func (m *NoopMessenger) Send(_ context.Context, _ *commonpb.ChatId, _ *messagingpb.Message) error {
	return nil
}
