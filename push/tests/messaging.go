package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/event"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/profile"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/push"
)

type mockPusher struct {
	lastChatID      *commonpb.ChatId
	lastPushMembers []*commonpb.UserId
	lastTitle       string
	lastBody        string
	lastSender      *string
	lastData        map[string]string
}

func (m *mockPusher) SendPushes(ctx context.Context, chatID *commonpb.ChatId, members []*commonpb.UserId, title, body string, sender *string, data map[string]string) error {
	m.lastChatID = chatID
	m.lastPushMembers = members
	m.lastTitle = title
	m.lastBody = body
	m.lastSender = sender
	m.lastData = data
	return nil
}

func (m *mockPusher) SendSilentPushes(ctx context.Context, chatID *commonpb.ChatId, members []*commonpb.UserId, data map[string]string) error {
	m.lastChatID = chatID
	m.lastPushMembers = members
	m.lastTitle = ""
	m.lastBody = ""
	m.lastSender = nil
	m.lastData = data
	return nil
}

func (m *mockPusher) reset() {
	m.lastChatID = nil
	m.lastPushMembers = nil
	m.lastTitle = ""
	m.lastBody = ""
	m.lastSender = nil
	m.lastData = nil
}

func RunMessagingTests(
	t *testing.T,
	pushes push.TokenStore,
	profiles profile.Store,
	chats chat.Store,
	teardown func(),
) {

	for _, tf := range []func(
		t *testing.T,
		pushes push.TokenStore,
		profiles profile.Store,
		chats chat.Store,
	){
		testEventHandler_HandleMessage,
	} {
		tf(t, pushes, profiles, chats)
		teardown()
	}
}

func testEventHandler_HandleMessage(t *testing.T, _ push.TokenStore, profileStore profile.Store, chatStore chat.Store) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	pusher := &mockPusher{}

	handler := push.NewPushEventHandler(logger, chatStore, profileStore, pusher)

	// Setup test data
	sender := &commonpb.UserId{Value: []byte("sender")}
	recipient := &commonpb.UserId{Value: []byte("recipient")}

	// Create profiles
	senderName := "Sender Name"
	require.NoError(t, profileStore.SetDisplayName(ctx, sender, "Sender Name"))

	tests := []struct {
		name           string
		setupChat      func() (*chatpb.Metadata, error)
		message        *messagingpb.Message
		expectedTitle  string
		expectedBody   string
		expectedSender *string
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
			expectedTitle:  senderName,
			expectedBody:   "Hello World",
			expectedSender: &senderName,
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
			expectedTitle:  "#2",
			expectedBody:   "Hello Group",
			expectedSender: &senderName,
			expectedPushes: []*commonpb.UserId{recipient},
		},
		{
			name: "group_chat_with_display_name_text_message",
			setupChat: func() (*chatpb.Metadata, error) {
				chatID := model.MustGenerateChatID()
				md, err := chatStore.CreateChat(ctx, &chatpb.Metadata{
					ChatId: chatID,
					Type:   chatpb.Metadata_GROUP,
				})
				if err != nil {
					return nil, err
				}

				err = chatStore.SetDisplayName(ctx, chatID, "Room Name")
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
			expectedTitle:  "#3: Room Name",
			expectedBody:   "Hello Group",
			expectedSender: &senderName,
			expectedPushes: []*commonpb.UserId{recipient},
		},
		{
			name: "push_disabled_recipient_no_push",
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
					err = chatStore.AddMember(ctx, chatID, chat.Member{UserID: user})
					if err != nil {
						return nil, err
					}

					err = chatStore.SetPushState(ctx, chatID, user, false)
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
				assert.Equal(t, tt.expectedSender, pusher.lastSender)
			}

			pusher.reset()
		})
	}
}
