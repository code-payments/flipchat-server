package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	"github.com/code-payments/flipchat-server/messaging"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/query"
)

func RunStoreTests(
	t *testing.T,
	ms messaging.MessageStore,
	ps messaging.PointerStore,
	teardown func(),
) {

	for _, tf := range []func(
		t *testing.T,
		ms messaging.MessageStore,
		ps messaging.PointerStore,
	){
		testMessageStore,
		testPointerStore,
	} {
		tf(t, ms, ps)
		teardown()
	}
}

func testMessageStore(t *testing.T, s messaging.MessageStore, _ messaging.PointerStore) {
	ctx := context.Background()
	chatID := model.MustGenerateChatID()

	t.Run("Empty", func(t *testing.T) {
		message, err := s.GetMessage(ctx, chatID, messaging.MustGenerateMessageID())
		require.Equal(t, messaging.ErrMessageNotFound, err)
		require.Nil(t, message)

		messages, err := s.GetPagedMessages(ctx, chatID)
		require.NoError(t, err)
		require.Empty(t, messages)

		unread, err := s.CountUnread(ctx, chatID, model.MustGenerateUserID(), nil, -1)
		require.NoError(t, err)
		require.Zero(t, unread)
	})

	var users []*commonpb.UserId
	for range 2 {
		users = append(users, model.MustGenerateUserID())
	}

	var messages []*messagingpb.Message
	var reversedMessages []*messagingpb.Message

	t.Run("Append", func(t *testing.T) {
		numMessages := 10
		for i := range numMessages {
			senders := append(users, nil)
			for _, sender := range senders {
				msg := &messagingpb.Message{
					SenderId: sender,
					Content: []*messagingpb.Content{
						{
							Type: &messagingpb.Content_Text{
								Text: &messagingpb.TextContent{
									Text: fmt.Sprintf("i: %d", i),
								},
							},
						},
					},
					WasSenderOffStage: i < numMessages/2,
				}

				// Ensure time ordering is progressing, otherwise ms collisions is
				// non-deterministic (well, won't be post sort, but this way we don't
				// have to sort)
				time.Sleep(time.Millisecond)

				// Randomly use PutMessage or PutMessageLegacy
				var fn func(context.Context, *commonpb.ChatId, *messagingpb.Message) (*messagingpb.Message, error)
				if i%2 == 0 {
					fn = s.PutMessage
				} else {
					fn = s.PutMessageLegacy
				}

				beforeMsg := proto.Clone(msg).(*messagingpb.Message)
				afterMsg, err := fn(ctx, chatID, beforeMsg)

				require.NoError(t, err)
				require.NotNil(t, afterMsg.MessageId)
				require.WithinDuration(t, time.Now(), afterMsg.Ts.AsTime(), time.Second)

				// Ensure there are no side effects (old behavior had side effects)
				require.NoError(t, protoutil.ProtoEqualError(beforeMsg, msg))

				messages = append(messages, afterMsg)
				reversedMessages = append([]*messagingpb.Message{afterMsg}, reversedMessages...)
			}
		}
	})

	t.Run("GetMessage", func(t *testing.T) {
		for _, message := range messages {
			actual, err := s.GetMessage(ctx, chatID, message.MessageId)
			require.NoError(t, err)
			require.NoError(t, protoutil.ProtoEqualError(message, actual))
		}
	})

	t.Run("GetBatchMessages", func(t *testing.T) {
		actual, err := s.GetBatchMessages(ctx, chatID, messaging.MustGenerateMessageID())
		require.NoError(t, err)
		require.Empty(t, actual)

		actual, err = s.GetBatchMessages(ctx, chatID, messages[0].MessageId, messages[1].MessageId, messaging.MustGenerateMessageID())
		require.NoError(t, err)
		require.Len(t, actual, 2)
		require.NoError(t, protoutil.SliceEqualError([]*messagingpb.Message{messages[0], messages[1]}, actual))
	})

	t.Run("GetPagedMessages", func(t *testing.T) {
		actual, err := s.GetPagedMessages(ctx, chatID)
		require.NoError(t, err)
		require.NoError(t, protoutil.SliceEqualError(messages, actual))

		actual, err = s.GetPagedMessages(
			ctx,
			chatID,
			query.WithOrder(commonpb.QueryOptions_DESC),
		)
		require.NoError(t, err)
		require.NoError(t, protoutil.SliceEqualError(reversedMessages, actual))

		actual, err = s.GetPagedMessages(
			ctx,
			chatID,
			query.WithLimit(5),
		)
		require.NoError(t, err)
		require.NoError(t, protoutil.SliceEqualError(messages[:5], actual))

		actual, err = s.GetPagedMessages(
			ctx,
			chatID,
			query.WithToken(&commonpb.PagingToken{Value: messages[12].MessageId.Value}),
		)
		require.NoError(t, err)
		require.NoError(t, protoutil.SliceEqualError(messages[13:], actual))

		actual, err = s.GetPagedMessages(
			ctx,
			chatID,
			query.WithToken(&commonpb.PagingToken{Value: messages[3].MessageId.Value}),
			query.WithOrder(commonpb.QueryOptions_DESC),
		)
		require.NoError(t, err)
		require.NoError(t, protoutil.SliceEqualError(reversedMessages[27:], actual))

		actual, err = s.GetPagedMessages(
			ctx,
			chatID,
			query.WithToken(&commonpb.PagingToken{Value: messages[15].MessageId.Value}),
			query.WithOrder(commonpb.QueryOptions_DESC),
			query.WithLimit(10),
		)
		require.NoError(t, err)
		require.NoError(t, protoutil.SliceEqualError(reversedMessages[15:25], actual))
	})

	// todo: tests for filtering message types that don't generate unreads
	t.Run("Unread", func(t *testing.T) {
		unread, err := s.CountUnread(ctx, chatID, users[0], nil, -1)
		require.NoError(t, err)
		require.EqualValues(t, 20, unread)

		unread, err = s.CountUnread(ctx, chatID, users[0], nil, 3)
		require.NoError(t, err)
		require.EqualValues(t, 3, unread)

		unread, err = s.CountUnread(ctx, chatID, users[0], messages[7].MessageId, -1)
		require.NoError(t, err)
		require.EqualValues(t, 15, unread)

		unread, err = s.CountUnread(ctx, chatID, users[0], messages[7].MessageId, 2)
		require.NoError(t, err)
		require.EqualValues(t, 2, unread)
	})
}

func testPointerStore(t *testing.T, _ messaging.MessageStore, s messaging.PointerStore) {
	ctx := context.Background()
	chatID := model.MustGenerateChatID()
	userID := model.MustGenerateUserID()

	t.Run("Empty", func(t *testing.T) {
		ptrs, err := s.GetAllPointers(ctx, chatID)
		require.NoError(t, err)
		require.Empty(t, ptrs)

		userPtrs, err := s.GetPointers(ctx, chatID, userID)
		require.NoError(t, err)
		require.Empty(t, userPtrs)
	})

	t.Run("Advance", func(t *testing.T) {
		var expectedPtrs []*messagingpb.Pointer
		var expectedAll []messaging.UserPointer
		for ptrType := messagingpb.Pointer_SENT; ptrType < messagingpb.Pointer_READ; ptrType++ {
			ptr := &messagingpb.Pointer{
				Type:  ptrType,
				Value: messaging.MustGenerateMessageID(),
			}

			advanced, err := s.AdvancePointer(ctx, chatID, userID, ptr)
			require.NoError(t, err)
			require.True(t, advanced)

			expectedPtrs = append(expectedPtrs, ptr)
			expectedAll = append(expectedAll, messaging.UserPointer{
				UserID:  userID,
				Pointer: ptr,
			})

			userPtrs, err := s.GetPointers(ctx, chatID, userID)
			require.NoError(t, err)
			require.NoError(t, protoutil.SliceEqualError(expectedPtrs, userPtrs))
		}

		for ptrType := messagingpb.Pointer_SENT; ptrType < messagingpb.Pointer_READ; ptrType++ {
			ptr := &messagingpb.Pointer{
				Type:  ptrType,
				Value: messaging.MustGenerateMessageIDFromTime(time.Now().Add(-time.Hour)),
			}

			advanced, err := s.AdvancePointer(ctx, chatID, userID, ptr)
			require.NoError(t, err)
			require.False(t, advanced)

			userPtrs, err := s.GetPointers(ctx, chatID, userID)
			require.NoError(t, err)
			require.NoError(t, protoutil.SliceEqualError(expectedPtrs, userPtrs))
		}

		allPtrs, err := s.GetAllPointers(ctx, chatID)
		require.NoError(t, err)
		require.Equal(t, len(expectedAll), len(allPtrs))

		for i := range allPtrs {
			require.NoError(t, protoutil.ProtoEqualError(expectedAll[i].UserID, allPtrs[i].UserID))
			require.NoError(t, protoutil.ProtoEqualError(expectedAll[i].Pointer, allPtrs[i].Pointer))
		}
	})
}
