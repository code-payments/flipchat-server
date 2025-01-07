package messaging

import (
	"context"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
)

type Messenger interface {
	Send(ctx context.Context, chatID *commonpb.ChatId, msg *messagingpb.Message) (*messagingpb.Message, error)
}

type NoopMessenger struct {
}

func NewNoopMessenger() *NoopMessenger {
	return &NoopMessenger{}
}

func (m *NoopMessenger) Send(_ context.Context, _ *commonpb.ChatId, msg *messagingpb.Message) (*messagingpb.Message, error) {
	msg = proto.Clone(msg).(*messagingpb.Message)

	msg.MessageId = MustGenerateMessageID()

	if msg.Ts == nil {
		msg.Ts = timestamppb.Now()
	}

	return msg, nil
}
