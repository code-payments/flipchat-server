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

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	codedata "github.com/code-payments/code-server/pkg/code/data"
	codekin "github.com/code-payments/code-server/pkg/kin"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/event"
	"github.com/code-payments/flipchat-server/intent"
	"github.com/code-payments/flipchat-server/messaging"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/testutil"
)

type testAuthn struct {
}

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
		NewAlwaysAllowRpcAuthz(),
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

	streamParams := &messagingpb.StreamMessagesRequest_Params{ChatId: chatID}
	require.NoError(t, keyPair.Auth(streamParams, &streamParams.Auth))

	stream, err := client.StreamMessages(ctx)
	require.NoError(t, err)

	require.NoError(t, stream.Send(&messagingpb.StreamMessagesRequest{
		Type: &messagingpb.StreamMessagesRequest_Params_{
			Params: streamParams,
		},
	}))

	eventCh := make(chan *messagingpb.StreamMessagesResponse_MessageBatch, 1024)
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
			require.NoError(t, keyPair.Auth(send, &send.Auth))

			sent, err := client.SendMessage(ctx, send)
			require.NoError(t, err)
			require.Equal(t, messagingpb.SendMessageResponse_OK, sent.Result)

			expected = append(expected, sent.Message)

			notification := <-eventCh
			require.NoError(t, protoutil.ProtoEqualError(sent.Message, notification.Messages[0]))
		}

		/*
			for i := range 10 {
				send := &messagingpb.SendMessageRequest{
					ChatId: chatID,
					Content: []*messagingpb.Content{
						{
							Type: &messagingpb.Content_Reaction{
								Reaction: &messagingpb.ReactionContent{
									OriginalMessageId: expected[i].MessageId,
									Emoji:             "👍",
								},
							},
						},
					},
				}
				require.NoError(t, keyPair.Auth(send, &send.Auth))

				sent, err := client.SendMessage(ctx, send)
				require.NoError(t, err)
				require.Equal(t, messagingpb.SendMessageResponse_OK, sent.Result)

				expected = append(expected, sent.Message)

				notification := <-eventCh
				require.NoError(t, protoutil.ProtoEqualError(sent.Message, notification.Messages[0]))
			}
		*/

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
			require.NoError(t, keyPair.Auth(send, &send.Auth))

			sent, err := client.SendMessage(ctx, send)
			require.NoError(t, err)
			require.Equal(t, messagingpb.SendMessageResponse_OK, sent.Result)

			expected = append(expected, sent.Message)

			notification := <-eventCh
			require.NoError(t, protoutil.ProtoEqualError(sent.Message, notification.Messages[0]))
		}

		for i := range 10 {
			tipAmount := codekin.ToQuarks(uint64(i + 1))
			tipPaymentMetadata := &messagingpb.SendTipMessagePaymentMetadata{
				ChatId:    chatID,
				MessageId: expected[i].MessageId,
				TipperId:  userID,
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
			require.NoError(t, keyPair.Auth(send, &send.Auth))

			sent, err := client.SendMessage(ctx, send)
			require.NoError(t, err)
			require.Equal(t, messagingpb.SendMessageResponse_OK, sent.Result)

			expected = append(expected, sent.Message)

			notification := <-eventCh
			require.NoError(t, protoutil.ProtoEqualError(sent.Message, notification.Messages[0]))
		}
	})

	t.Run("GetMessage", func(t *testing.T) {
		get := &messagingpb.GetMessageRequest{
			ChatId:    chatID,
			MessageId: expected[0].MessageId,
		}
		require.NoError(t, keyPair.Auth(get, &get.Auth))

		message, err := client.GetMessage(ctx, get)
		require.NoError(t, err)
		require.Equal(t, messagingpb.GetMessageResponse_OK, message.Result)
		require.NoError(t, protoutil.ProtoEqualError(expected[0], message.Message))
	})

	t.Run("GetMessages", func(t *testing.T) {
		get := &messagingpb.GetMessagesRequest{
			ChatId: chatID,
		}
		require.NoError(t, keyPair.Auth(get, &get.Auth))

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
			require.NoError(t, keyPair.Auth(notify, &notify.Auth))

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
			require.NoError(t, keyPair.Auth(advance, &advance.Auth))

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
		NewAlwaysAllowRpcAuthz(),
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
