package messaging

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

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

func SendStatusMessage(ctx context.Context, messenger Messenger, chatID *commonpb.ChatId, contentBuilder StatusMessageContentBuilder) error {
	content, err := contentBuilder()
	if err != nil {
		return err
	}
	msg := &messagingpb.Message{
		Content: []*messagingpb.Content{
			{Type: &messagingpb.Content_LocalizedStatus{LocalizedStatus: content}},
		},
		Ts: timestamppb.Now(),
	}
	return messenger.Send(ctx, chatID, msg)
}
