package push

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	memChat "github.com/code-payments/flipchat-server/chat/memory"
	memProfile "github.com/code-payments/flipchat-server/profile/memory"

	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/event"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"
)

type mockPusher struct {
	lastChatID      *commonpb.ChatId
	lastPushMembers []*commonpb.UserId
	lastTitle       string
	lastBody        string
	lastData        map[string]string
}

func (m *mockPusher) SendPushes(ctx context.Context, chatID *commonpb.ChatId, members []*commonpb.UserId, title, body string, data map[string]string) error {
	m.lastChatID = chatID
	m.lastPushMembers = members
	m.lastTitle = title
	m.lastBody = body
	m.lastData = data
	return nil
}

func (m *mockPusher) reset() {
	m.lastPushMembers = nil
	m.lastTitle = ""
	m.lastBody = ""
}

func TestEventHandler_HandleMessage(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	chatStore := memChat.NewInMemory()
	profileStore := memProfile.NewInMemory()
	pusher := &mockPusher{}

	handler := NewPushEventHandler(logger, chatStore, profileStore, pusher)

	// Setup test data
	sender := &commonpb.UserId{Value: []byte("sender")}
	recipient := &commonpb.UserId{Value: []byte("recipient")}

	// Create profiles
	require.NoError(t, profileStore.SetDisplayName(ctx, sender, "Sender Name"))

	tests := []struct {
		name           string
		setupChat      func() (*chatpb.Metadata, error)
		message        *messagingpb.Message
		expectedTitle  string
		expectedBody   string
		expectedPushes []*commonpb.UserId
	}{
		{
			name: "two_way_chat_text_message",
			setupChat: func() (*chatpb.Metadata, error) {
				chatID := model.MustGenerateTwoWayChatID(sender, recipient)

				md, err := chatStore.CreateChat(ctx, &chatpb.Metadata{
					ChatId: chatID,
					Type:   chatpb.Metadata_TWO_WAY,
				})
				if err != nil {
					return nil, err
				}

				for _, user := range []*commonpb.UserId{sender, recipient} {
					err = chatStore.AddMember(ctx, chatID, chat.Member{UserID: user})
					if err != nil {
						return nil, err
					}
				}

				return md, nil
			},
			message: &messagingpb.Message{
				SenderId: sender,
				Content: []*messagingpb.Content{
					{
						Type: &messagingpb.Content_Text{
							Text: &messagingpb.TextContent{
								Text: "Hello World",
							},
						},
					},
				},
			},
			expectedTitle:  "Sender Name",
			expectedBody:   "Hello World",
			expectedPushes: []*commonpb.UserId{recipient},
		},
		{
			name: "group_chat_text_message",
			setupChat: func() (*chatpb.Metadata, error) {
				chatID := model.MustGenerateChatID()
				md, err := chatStore.CreateChat(ctx, &chatpb.Metadata{
					ChatId: chatID,
					Type:   chatpb.Metadata_GROUP,
				})
				if err != nil {
					return nil, err
				}

				for _, user := range []*commonpb.UserId{sender, recipient} {
					err = chatStore.AddMember(ctx, chatID, chat.Member{UserID: user})
					if err != nil {
						return nil, err
					}
				}

				return md, nil
			},
			message: &messagingpb.Message{
				SenderId: sender,
				Content: []*messagingpb.Content{
					{
						Type: &messagingpb.Content_Text{
							Text: &messagingpb.TextContent{
								Text: "Hello Group",
							},
						},
					},
				},
			},
			expectedTitle:  "Room #2",
			expectedBody:   "Sender Name: Hello Group",
			expectedPushes: []*commonpb.UserId{recipient},
		},
		{
			name: "muted_recipient_no_push",
			setupChat: func() (*chatpb.Metadata, error) {
				chatID := model.MustGenerateChatID()
				md, err := chatStore.CreateChat(ctx, &chatpb.Metadata{
					ChatId: chatID,
					Type:   chatpb.Metadata_TWO_WAY,
				})
				if err != nil {
					return nil, err
				}

				for _, user := range []*commonpb.UserId{sender, recipient} {
					err = chatStore.AddMember(ctx, chatID, chat.Member{UserID: user, HasMuted: true})
					if err != nil {
						return nil, err
					}
				}

				return md, nil
			},
			message: &messagingpb.Message{
				SenderId: sender,
				Content: []*messagingpb.Content{
					{
						Type: &messagingpb.Content_Text{
							Text: &messagingpb.TextContent{
								Text: "Hello Muted",
							},
						},
					},
				},
			},
			expectedPushes: nil,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md, err := tt.setupChat()
			require.NoError(t, err)

			handler.OnEvent(md.ChatId, &event.ChatEvent{
				MessageUpdate: tt.message,
			})

			require.NoError(t, protoutil.SliceEqualError(tt.expectedPushes, pusher.lastPushMembers), "i: %d", i)
			if tt.expectedPushes != nil {
				assert.Equal(t, tt.expectedTitle, pusher.lastTitle)
				assert.Equal(t, tt.expectedBody, pusher.lastBody)
				assert.Equal(t, base64.StdEncoding.EncodeToString(md.ChatId.Value), pusher.lastData["chat_id"])
			}

			pusher.reset()
		})
	}
}
