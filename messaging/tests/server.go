package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	codedata "github.com/code-payments/code-server/pkg/code/data"
	codekin "github.com/code-payments/code-server/pkg/kin"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	auth_rpc "github.com/code-payments/flipchat-server/auth/rpc"
	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/event"
	"github.com/code-payments/flipchat-server/intent"
	"github.com/code-payments/flipchat-server/messaging"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/testutil"
)

// todo: needs authz tests
func RunServerTests(
	t *testing.T,
	accounts account.Store,
	intents intent.Store,
	messages messaging.MessageStore,
	pointers messaging.PointerStore,
	chats chat.Store,
	teardown func(),
) {

	for _, tf := range []func(
		t *testing.T,
		accounts account.Store,
		intents intent.Store,
		messages messaging.MessageStore,
		pointers messaging.PointerStore,
		chats chat.Store,
	){
		testServerHappy,
		testServerDuplicateStreams,
	} {
		tf(t, accounts, intents, messages, pointers, chats)
		teardown()
	}
}

func testServerHappy(
	t *testing.T,
	accountStore account.Store,
	intents intent.Store,
	messageDB messaging.MessageStore,
	pointerDB messaging.PointerStore,
	chatsDB chat.Store,
) {
	log := zap.Must(zap.NewDevelopment())
	authz := account.NewAuthorizer(log, accountStore, auth.NewKeyPairAuthenticator())
	bus := event.NewBus[*commonpb.ChatId, *event.ChatEvent](func(id *commonpb.ChatId) []byte {
		return id.Value
	})

	codeData := codedata.NewTestDataProvider()

	serv := messaging.NewServer(
		log,
		authz,
		auth_rpc.NewMessagingRpcAuthorizer(chatsDB, intents, messageDB, codeData),
		accountStore,
		intents,
		messageDB,
		pointerDB,
		codeData,
		bus,
	)

	cc := testutil.RunGRPCServer(t, testutil.WithService(func(s *grpc.Server) {
		messagingpb.RegisterMessagingServer(s, serv)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := messagingpb.NewMessagingClient(cc)

	chatID := model.MustGenerateChatID()
	ownerUserID := model.MustGenerateUserID()
	otherUserID := model.MustGenerateUserID()
	ownerKeyPair := model.MustGenerateKeyPair()
	otherKeyPair := model.MustGenerateKeyPair()
	_, _ = accountStore.Bind(ctx, ownerUserID, ownerKeyPair.Proto())
	_, _ = accountStore.Bind(ctx, otherUserID, otherKeyPair.Proto())
	_, err := chatsDB.CreateChat(ctx, &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
		Owner:  ownerUserID,
	})
	require.NoError(t, err)
	require.NoError(t, chatsDB.AddMember(ctx, chatID, chat.Member{
		UserID:            ownerUserID,
		HasSendPermission: true,
	}))
	require.NoError(t, chatsDB.AddMember(ctx, chatID, chat.Member{
		UserID:            otherUserID,
		HasSendPermission: true,
	}))

	streamParams := &messagingpb.StreamMessagesRequest_Params{ChatId: chatID}
	require.NoError(t, ownerKeyPair.Auth(streamParams, &streamParams.Auth))

	stream, err := client.StreamMessages(ctx)
	require.NoError(t, err)

	require.NoError(t, stream.Send(&messagingpb.StreamMessagesRequest{
		Type: &messagingpb.StreamMessagesRequest_Params_{
			Params: streamParams,
		},
	}))

	eventCh := make(chan *messagingpb.MessageBatch, 1024)
	go func() {
		defer close(eventCh)
		for {
			resp, err := stream.Recv()
			if status.Code(err) == codes.Canceled {
				return
			}
			require.NoError(t, err)

			switch t := resp.Type.(type) {
			case *messagingpb.StreamMessagesResponse_Ping:
				_ = stream.Send(&messagingpb.StreamMessagesRequest{
					Type: &messagingpb.StreamMessagesRequest_Pong{
						Pong: &commonpb.ClientPong{
							Timestamp: timestamppb.Now(),
						},
					},
				})
			case *messagingpb.StreamMessagesResponse_Error:
				log.Warn("Failure in stream", zap.Any("err", t.Error))
			case *messagingpb.StreamMessagesResponse_Messages:
				eventCh <- t.Messages
			}
		}
	}()

	// Note: It's possible flush message picks up the first few messages.
	time.Sleep(500 * time.Millisecond)

	var expected []*messagingpb.Message
	var expectedTextMessages []*messagingpb.Message
	var expectedReplyMessages []*messagingpb.Message
	var expectedReactionMessages []*messagingpb.Message
	var expectedTipMessages []*messagingpb.Message
	var expectedDeletedMessages []*messagingpb.Message
	var expectedReviewMessages []*messagingpb.Message
	t.Run("Send Messages", func(t *testing.T) {
		for i := range 10 {
			send := &messagingpb.SendMessageRequest{
				ChatId: chatID,
				Content: []*messagingpb.Content{
					{
						Type: &messagingpb.Content_Text{
							Text: &messagingpb.TextContent{Text: fmt.Sprintf("msg-%d", i)},
						},
					},
				},
			}
			require.NoError(t, ownerKeyPair.Auth(send, &send.Auth))

			sent, err := client.SendMessage(ctx, send)
			require.NoError(t, err)
			require.Equal(t, messagingpb.SendMessageResponse_OK, sent.Result)
			require.False(t, sent.Message.WasSenderOffStage)

			expected = append(expected, sent.Message)
			expectedTextMessages = append(expectedTextMessages, sent.Message)

			notification := <-eventCh
			require.NoError(t, protoutil.ProtoEqualError(sent.Message, notification.Messages[0]))
		}

		for i := range 10 {
			send := &messagingpb.SendMessageRequest{
				ChatId: chatID,
				Content: []*messagingpb.Content{
					{
						Type: &messagingpb.Content_Reply{
							Reply: &messagingpb.ReplyContent{
								OriginalMessageId: expected[i].MessageId,
								ReplyText:         fmt.Sprintf("reply-%d", i),
							},
						},
					},
				},
			}
			require.NoError(t, ownerKeyPair.Auth(send, &send.Auth))

			sent, err := client.SendMessage(ctx, send)
			require.NoError(t, err)
			require.Equal(t, messagingpb.SendMessageResponse_OK, sent.Result)
			require.False(t, sent.Message.WasSenderOffStage)

			expected = append(expected, sent.Message)
			expectedReplyMessages = append(expectedReplyMessages, sent.Message)

			notification := <-eventCh
			require.NoError(t, protoutil.ProtoEqualError(sent.Message, notification.Messages[0]))
		}

		for _, reference := range append(expectedTextMessages, expectedReplyMessages...) {
			send := &messagingpb.SendMessageRequest{
				ChatId: chatID,
				Content: []*messagingpb.Content{
					{
						Type: &messagingpb.Content_Reaction{
							Reaction: &messagingpb.ReactionContent{
								OriginalMessageId: reference.MessageId,
								Emoji:             "👍",
							},
						},
					},
				},
			}
			require.NoError(t, ownerKeyPair.Auth(send, &send.Auth))

			sent, err := client.SendMessage(ctx, send)
			require.NoError(t, err)
			require.Equal(t, messagingpb.SendMessageResponse_OK, sent.Result)
			require.False(t, sent.Message.WasSenderOffStage)

			expected = append(expected, sent.Message)
			expectedReactionMessages = append(expectedReactionMessages, sent.Message)

			notification := <-eventCh
			require.NoError(t, protoutil.ProtoEqualError(sent.Message, notification.Messages[0]))
		}

		for i, reference := range append(expectedTextMessages, expectedReplyMessages...) {
			tipAmount := codekin.ToQuarks(uint64(i + 1))
			tipPaymentMetadata := &messagingpb.SendTipMessagePaymentMetadata{
				ChatId:    chatID,
				MessageId: reference.MessageId,
				TipperId:  ownerUserID,
			}
			tipIntentID := testutil.CreatePayment(t, codeData, tipAmount, tipPaymentMetadata)

			send := &messagingpb.SendMessageRequest{
				ChatId: chatID,
				Content: []*messagingpb.Content{
					{
						Type: &messagingpb.Content_Tip{
							Tip: &messagingpb.TipContent{
								OriginalMessageId: expected[i].MessageId,
								TipAmount:         &commonpb.PaymentAmount{Quarks: tipAmount},
							},
						},
					},
				},
				PaymentIntent: tipIntentID,
			}
			require.NoError(t, ownerKeyPair.Auth(send, &send.Auth))

			sent, err := client.SendMessage(ctx, send)
			require.NoError(t, err)
			require.Equal(t, messagingpb.SendMessageResponse_OK, sent.Result)
			require.False(t, sent.Message.WasSenderOffStage)

			expected = append(expected, sent.Message)
			expectedTipMessages = append(expectedTipMessages, sent.Message)

			notification := <-eventCh
			require.NoError(t, protoutil.ProtoEqualError(sent.Message, notification.Messages[0]))
		}

		for _, reference := range append(expectedTextMessages, append(expectedReplyMessages, expectedReactionMessages...)...) {
			send := &messagingpb.SendMessageRequest{
				ChatId: chatID,
				Content: []*messagingpb.Content{
					{
						Type: &messagingpb.Content_Deleted{
							Deleted: &messagingpb.DeleteMessageContent{
								OriginalMessageId: reference.MessageId,
							},
						},
					},
				},
			}
			require.NoError(t, ownerKeyPair.Auth(send, &send.Auth))

			sent, err := client.SendMessage(ctx, send)
			require.NoError(t, err)
			require.Equal(t, messagingpb.SendMessageResponse_OK, sent.Result)
			require.False(t, sent.Message.WasSenderOffStage)

			expected = append(expected, sent.Message)
			expectedDeletedMessages = append(expectedDeletedMessages, sent.Message)

			notification := <-eventCh
			require.NoError(t, protoutil.ProtoEqualError(sent.Message, notification.Messages[0]))
		}

		t.Run("Send message with invalid reference", func(t *testing.T) {
			contentsWithReference := [][]*messagingpb.Content{
				{
					{
						Type: &messagingpb.Content_Reaction{
							Reaction: &messagingpb.ReactionContent{
								OriginalMessageId: messaging.MustGenerateMessageID(),
								Emoji:             "👎",
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Reaction{
							Reaction: &messagingpb.ReactionContent{
								OriginalMessageId: expectedReactionMessages[0].MessageId,
								Emoji:             "👎",
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Reaction{
							Reaction: &messagingpb.ReactionContent{
								OriginalMessageId: expectedTipMessages[0].MessageId,
								Emoji:             "👎",
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Reaction{
							Reaction: &messagingpb.ReactionContent{
								OriginalMessageId: expectedDeletedMessages[0].MessageId,
								Emoji:             "👎",
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Reply{
							Reply: &messagingpb.ReplyContent{
								OriginalMessageId: messaging.MustGenerateMessageID(),
								ReplyText:         "invald-reply",
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Reply{
							Reply: &messagingpb.ReplyContent{
								OriginalMessageId: expectedReactionMessages[0].MessageId,
								ReplyText:         "invald-reply",
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Reply{
							Reply: &messagingpb.ReplyContent{
								OriginalMessageId: expectedTipMessages[0].MessageId,
								ReplyText:         "invald-reply",
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Reply{
							Reply: &messagingpb.ReplyContent{
								OriginalMessageId: expectedDeletedMessages[0].MessageId,
								ReplyText:         "invald-reply",
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Tip{
							Tip: &messagingpb.TipContent{
								OriginalMessageId: messaging.MustGenerateMessageID(),
								TipAmount:         &commonpb.PaymentAmount{Quarks: codekin.ToQuarks(1)},
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Tip{
							Tip: &messagingpb.TipContent{
								OriginalMessageId: expectedReactionMessages[0].MessageId,
								TipAmount:         &commonpb.PaymentAmount{Quarks: codekin.ToQuarks(1)},
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Tip{
							Tip: &messagingpb.TipContent{
								OriginalMessageId: expectedTipMessages[0].MessageId,
								TipAmount:         &commonpb.PaymentAmount{Quarks: codekin.ToQuarks(1)},
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Tip{
							Tip: &messagingpb.TipContent{
								OriginalMessageId: expectedDeletedMessages[0].MessageId,
								TipAmount:         &commonpb.PaymentAmount{Quarks: codekin.ToQuarks(1)},
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Deleted{
							Deleted: &messagingpb.DeleteMessageContent{
								OriginalMessageId: messaging.MustGenerateMessageID(),
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Deleted{
							Deleted: &messagingpb.DeleteMessageContent{
								OriginalMessageId: expectedTipMessages[0].MessageId,
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Deleted{
							Deleted: &messagingpb.DeleteMessageContent{
								OriginalMessageId: expectedDeletedMessages[0].MessageId,
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Review{
							Review: &messagingpb.ReviewContent{
								OriginalMessageId: messaging.MustGenerateMessageID(),
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Review{
							Review: &messagingpb.ReviewContent{
								OriginalMessageId: expectedTextMessages[0].MessageId,
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Review{
							Review: &messagingpb.ReviewContent{
								OriginalMessageId: expectedReplyMessages[0].MessageId,
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Review{
							Review: &messagingpb.ReviewContent{
								OriginalMessageId: expectedReactionMessages[0].MessageId,
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Review{
							Review: &messagingpb.ReviewContent{
								OriginalMessageId: expectedTipMessages[0].MessageId,
							},
						},
					},
				},
				{
					{
						Type: &messagingpb.Content_Review{
							Review: &messagingpb.ReviewContent{
								OriginalMessageId: expectedDeletedMessages[0].MessageId,
							},
						},
					},
				},
			}
			for _, content := range contentsWithReference {
				send := &messagingpb.SendMessageRequest{
					ChatId:  chatID,
					Content: content,
				}
				require.NoError(t, ownerKeyPair.Auth(send, &send.Auth))
				sent, err := client.SendMessage(ctx, send)
				require.NoError(t, err)
				require.Equal(t, messagingpb.SendMessageResponse_DENIED, sent.Result)
			}
		})

		t.Run("Send tip message with invalid intent", func(t *testing.T) {
			invalidIntentCtors := []func() *commonpb.IntentId{
				func() *commonpb.IntentId {
					return nil
				},
				func() *commonpb.IntentId {
					return &commonpb.IntentId{Value: make([]byte, 32)}
				},
				func() *commonpb.IntentId {
					tipPaymentMetadata := &messagingpb.SendTipMessagePaymentMetadata{
						ChatId:    chatID,
						MessageId: expectedTextMessages[0].MessageId,
						TipperId:  ownerUserID,
					}
					return testutil.CreatePayment(t, codeData, codekin.ToQuarks(1)+1, tipPaymentMetadata)
				},
				func() *commonpb.IntentId {
					tipPaymentMetadata := &messagingpb.SendTipMessagePaymentMetadata{
						ChatId:    model.MustGenerateChatID(),
						MessageId: expectedTextMessages[0].MessageId,
						TipperId:  ownerUserID,
					}
					return testutil.CreatePayment(t, codeData, codekin.ToQuarks(1), tipPaymentMetadata)
				},
				func() *commonpb.IntentId {
					tipPaymentMetadata := &messagingpb.SendTipMessagePaymentMetadata{
						ChatId:    chatID,
						MessageId: expectedTextMessages[1].MessageId,
						TipperId:  ownerUserID,
					}
					return testutil.CreatePayment(t, codeData, codekin.ToQuarks(1), tipPaymentMetadata)
				},
				func() *commonpb.IntentId {
					tipPaymentMetadata := &messagingpb.SendTipMessagePaymentMetadata{
						ChatId:    chatID,
						MessageId: expectedTextMessages[0].MessageId,
						TipperId:  model.MustGenerateUserID(),
					}
					return testutil.CreatePayment(t, codeData, codekin.ToQuarks(1), tipPaymentMetadata)
				},
			}

			for _, tc := range invalidIntentCtors {
				send := &messagingpb.SendMessageRequest{
					ChatId: chatID,
					Content: []*messagingpb.Content{
						{
							Type: &messagingpb.Content_Tip{
								Tip: &messagingpb.TipContent{
									OriginalMessageId: expectedTextMessages[0].MessageId,
									TipAmount:         &commonpb.PaymentAmount{Quarks: codekin.ToQuarks(1)},
								},
							},
						},
					},
					PaymentIntent: tc(),
				}
				require.NoError(t, ownerKeyPair.Auth(send, &send.Auth))

				sent, err := client.SendMessage(ctx, send)
				require.NoError(t, err)
				require.Equal(t, messagingpb.SendMessageResponse_DENIED, sent.Result)
			}
		})

		t.Run("Send message to closed room", func(t *testing.T) {
			require.NoError(t, chatsDB.SetOpenStatus(ctx, chatID, false))

			send := &messagingpb.SendMessageRequest{
				ChatId: chatID,
				Content: []*messagingpb.Content{
					{
						Type: &messagingpb.Content_Text{
							Text: &messagingpb.TextContent{Text: "msg"},
						},
					},
				},
			}

			// Test other user cannot send certain types of messages (eg. text)
			require.NoError(t, otherKeyPair.Auth(send, &send.Auth))
			sent, err := client.SendMessage(ctx, send)
			require.NoError(t, err)
			require.Equal(t, messagingpb.SendMessageResponse_DENIED, sent.Result)

			// However, owner is always allowed
			require.NoError(t, ownerKeyPair.Auth(send, &send.Auth))
			sent, err = client.SendMessage(ctx, send)
			require.NoError(t, err)
			require.Equal(t, messagingpb.SendMessageResponse_OK, sent.Result)
			<-eventCh
			expected = append(expected, sent.Message)
			expectedTextMessages = append(expectedTextMessages, sent.Message)

			require.NoError(t, chatsDB.SetOpenStatus(ctx, chatID, true))

			send = &messagingpb.SendMessageRequest{
				ChatId: chatID,
				Content: []*messagingpb.Content{
					{
						Type: &messagingpb.Content_Reaction{
							Reaction: &messagingpb.ReactionContent{
								OriginalMessageId: sent.Message.MessageId,
								Emoji:             "👍",
							},
						},
					},
				},
			}

			// Test other user can send certain types of messages (eg. reaction)
			require.NoError(t, otherKeyPair.Auth(send, &send.Auth))
			sent, err = client.SendMessage(ctx, send)
			require.NoError(t, err)
			require.Equal(t, messagingpb.SendMessageResponse_OK, sent.Result)
			<-eventCh
			expected = append(expected, sent.Message)
			expectedReactionMessages = append(expectedReactionMessages, sent.Message)
		})

		t.Run("Send message when off stage with reviews", func(t *testing.T) {
			listenerUserID := model.MustGenerateUserID()
			listenerKeyPair := model.MustGenerateKeyPair()
			_, _ = accountStore.Bind(ctx, listenerUserID, listenerKeyPair.Proto())
			require.NoError(t, chatsDB.AddMember(ctx, chatID, chat.Member{
				UserID:            listenerUserID,
				HasSendPermission: false,
			}))

			sendAsListenerPaymentMetadata := &messagingpb.SendMessageAsListenerPaymentMetadata{
				ChatId: chatID,
				UserId: listenerUserID,
			}

			send := &messagingpb.SendMessageRequest{
				ChatId: chatID,
				Content: []*messagingpb.Content{
					{
						Type: &messagingpb.Content_Text{
							Text: &messagingpb.TextContent{Text: "msg-off-stage"},
						},
					},
				},
			}
			require.NoError(t, listenerKeyPair.Auth(send, &send.Auth))

			sent, err := client.SendMessage(ctx, send)
			require.NoError(t, err)
			require.Equal(t, messagingpb.SendMessageResponse_DENIED, sent.Result)

			send = &messagingpb.SendMessageRequest{
				ChatId: chatID,
				Content: []*messagingpb.Content{
					{
						Type: &messagingpb.Content_Text{
							Text: &messagingpb.TextContent{Text: "msg-off-stage"},
						},
					},
				},
				PaymentIntent: testutil.CreatePayment(t, codeData, codekin.ToQuarks(1), sendAsListenerPaymentMetadata),
			}
			require.NoError(t, listenerKeyPair.Auth(send, &send.Auth))

			sent, err = client.SendMessage(ctx, send)
			require.NoError(t, err)
			require.Equal(t, messagingpb.SendMessageResponse_OK, sent.Result)
			require.True(t, sent.Message.WasSenderOffStage)

			expected = append(expected, sent.Message)
			expectedTextMessages = append(expectedTextMessages, sent.Message)

			notification := <-eventCh
			require.NoError(t, protoutil.ProtoEqualError(sent.Message, notification.Messages[0]))

			send = &messagingpb.SendMessageRequest{
				ChatId: chatID,
				Content: []*messagingpb.Content{
					{
						Type: &messagingpb.Content_Reply{
							Reply: &messagingpb.ReplyContent{
								OriginalMessageId: sent.Message.MessageId,
								ReplyText:         "reply-off-stage",
							},
						},
					},
				},
				PaymentIntent: testutil.CreatePayment(t, codeData, codekin.ToQuarks(1), sendAsListenerPaymentMetadata),
			}
			require.NoError(t, listenerKeyPair.Auth(send, &send.Auth))

			sent, err = client.SendMessage(ctx, send)
			require.NoError(t, err)
			require.Equal(t, messagingpb.SendMessageResponse_OK, sent.Result)
			require.True(t, sent.Message.WasSenderOffStage)

			expected = append(expected, sent.Message)
			expectedReplyMessages = append(expectedReplyMessages, sent.Message)

			notification = <-eventCh
			require.NoError(t, protoutil.ProtoEqualError(sent.Message, notification.Messages[0]))

			for _, reference := range append(expectedTextMessages, expectedReplyMessages...) {
				if reference.WasSenderOffStage {
					send := &messagingpb.SendMessageRequest{
						ChatId: chatID,
						Content: []*messagingpb.Content{
							{
								Type: &messagingpb.Content_Review{
									Review: &messagingpb.ReviewContent{
										OriginalMessageId: reference.MessageId,
										IsApproved:        true,
									},
								},
							},
						},
					}
					require.NoError(t, ownerKeyPair.Auth(send, &send.Auth))

					sent, err := client.SendMessage(ctx, send)
					require.NoError(t, err)
					require.Equal(t, messagingpb.SendMessageResponse_OK, sent.Result)
					require.False(t, sent.Message.WasSenderOffStage)

					expected = append(expected, sent.Message)
					expectedReviewMessages = append(expectedReviewMessages, sent.Message)

					notification := <-eventCh
					require.NoError(t, protoutil.ProtoEqualError(sent.Message, notification.Messages[0]))
				}
			}
		})
	})

	t.Run("GetMessage", func(t *testing.T) {
		get := &messagingpb.GetMessageRequest{
			ChatId:    chatID,
			MessageId: expected[0].MessageId,
		}
		require.NoError(t, ownerKeyPair.Auth(get, &get.Auth))

		message, err := client.GetMessage(ctx, get)
		require.NoError(t, err)
		require.Equal(t, messagingpb.GetMessageResponse_OK, message.Result)
		require.NoError(t, protoutil.ProtoEqualError(expected[0], message.Message))
	})

	t.Run("GetMessages Batch", func(t *testing.T) {
		get := &messagingpb.GetMessagesRequest{
			ChatId: chatID,
			Query: &messagingpb.GetMessagesRequest_MessageIds{
				MessageIds: &messagingpb.MessageIdBatch{
					MessageIds: []*messagingpb.MessageId{expected[0].MessageId, expected[1].MessageId, messaging.MustGenerateMessageID()},
				},
			},
		}
		require.NoError(t, ownerKeyPair.Auth(get, &get.Auth))

		messages, err := client.GetMessages(ctx, get)
		require.NoError(t, err)
		require.Equal(t, messagingpb.GetMessagesResponse_OK, messages.Result)
		require.NoError(t, protoutil.SliceEqualError([]*messagingpb.Message{expected[0], expected[1]}, messages.Messages))
	})

	t.Run("GetMessages Paging", func(t *testing.T) {
		get := &messagingpb.GetMessagesRequest{
			ChatId: chatID,
			Query: &messagingpb.GetMessagesRequest_Options{
				Options: &commonpb.QueryOptions{
					PageSize: 1024,
					Order:    commonpb.QueryOptions_ASC,
				},
			},
		}
		require.NoError(t, ownerKeyPair.Auth(get, &get.Auth))

		messages, err := client.GetMessages(ctx, get)
		require.NoError(t, err)
		require.Equal(t, messagingpb.GetMessagesResponse_OK, messages.Result)
		require.NoError(t, protoutil.SliceEqualError(expected, messages.Messages))

		// todo: test paging parameters, but those are well-covered in store tests
	})

	t.Run("Notify Typing", func(t *testing.T) {
		for _, isTyping := range []bool{true, false} {
			notify := &messagingpb.NotifyIsTypingRequest{
				ChatId:   chatID,
				IsTyping: isTyping,
			}
			require.NoError(t, ownerKeyPair.Auth(notify, &notify.Auth))

			resp, err := client.NotifyIsTyping(ctx, notify)
			require.NoError(t, err)
			require.Equal(t, messagingpb.NotifyIsTypingResponse_OK, resp.Result)
		}
	})

	t.Run("Advance Pointer", func(t *testing.T) {
		for pt := messagingpb.Pointer_SENT; pt <= messagingpb.Pointer_READ; pt++ {
			advance := &messagingpb.AdvancePointerRequest{
				ChatId: chatID,
				Pointer: &messagingpb.Pointer{
					Type:  pt,
					Value: expected[4].MessageId,
				},
			}
			require.NoError(t, ownerKeyPair.Auth(advance, &advance.Auth))

			resp, err := client.AdvancePointer(ctx, advance)
			require.NoError(t, err)
			require.Equal(t, messagingpb.AdvancePointerResponse_OK, resp.Result)
		}
	})

	t.Run("No other notifications", func(t *testing.T) {
		select {
		case <-eventCh:
			t.Fatal("Should not have received other events")
		case <-time.After(500 * time.Millisecond):
		}
	})
}

func testServerDuplicateStreams(
	t *testing.T,
	accountStore account.Store,
	intents intent.Store,
	messageDB messaging.MessageStore,
	pointerDB messaging.PointerStore,
	chatsDB chat.Store,
) {
	log := zap.Must(zap.NewDevelopment())
	authz := account.NewAuthorizer(log, accountStore, auth.NewKeyPairAuthenticator())
	bus := event.NewBus[*commonpb.ChatId, *event.ChatEvent](func(id *commonpb.ChatId) []byte {
		return id.Value
	})
	codeData := codedata.NewTestDataProvider()

	serv := messaging.NewServer(
		log,
		authz,
		auth_rpc.NewMessagingRpcAuthorizer(chatsDB, intents, messageDB, codeData),
		accountStore,
		intents,
		messageDB,
		pointerDB,
		codeData,
		bus,
	)

	cc := testutil.RunGRPCServer(t, testutil.WithService(func(s *grpc.Server) {
		messagingpb.RegisterMessagingServer(s, serv)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := messagingpb.NewMessagingClient(cc)

	chatID := model.MustGenerateChatID()
	userID := model.MustGenerateUserID()
	keyPair := model.MustGenerateKeyPair()
	_, _ = accountStore.Bind(ctx, userID, keyPair.Proto())
	_, err := chatsDB.CreateChat(ctx, &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)
	require.NoError(t, chatsDB.AddMember(ctx, chatID, chat.Member{
		UserID:            userID,
		HasSendPermission: true,
	}))

	streamParams := &messagingpb.StreamMessagesRequest_Params{ChatId: chatID}
	require.NoError(t, keyPair.Auth(streamParams, &streamParams.Auth))

	streamA, err := client.StreamMessages(ctx)
	require.NoError(t, err)

	err = streamA.Send(&messagingpb.StreamMessagesRequest{Type: &messagingpb.StreamMessagesRequest_Params_{Params: streamParams}})
	require.NoError(t, err)
	_, err = streamA.Recv()
	require.NoError(t, err)

	streamB, err := client.StreamMessages(ctx)
	require.NoError(t, err)

	err = streamB.Send(&messagingpb.StreamMessagesRequest{Type: &messagingpb.StreamMessagesRequest_Params_{Params: streamParams}})
	require.NoError(t, err)
	_, err = streamB.Recv()
	require.NoError(t, err)

	_, err = streamA.Recv()
	require.Equal(t, codes.Aborted, status.Code(err))
}
