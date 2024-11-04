package messaging

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"
)

func TestPointerStore(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	chatID := model.MustGenerateChatID()
	userID := account.MustGenerateUserID()

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
		var expectedAll []UserPointer
		for ptrType := messagingpb.Pointer_SENT; ptrType < messagingpb.Pointer_READ; ptrType++ {
			ptr := &messagingpb.Pointer{
				Type:  ptrType,
				Value: MustGenerateMessageID(),
			}

			advanced, err := s.AdvancePointer(ctx, chatID, userID, ptr)
			require.NoError(t, err)
			require.True(t, advanced)

			expectedPtrs = append(expectedPtrs, ptr)
			expectedAll = append(expectedAll, UserPointer{
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
				Value: MustGenerateMessageIDFromTime(time.Now().Add(-time.Hour)),
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

func TestMessageStore(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	chatID := model.MustGenerateChatID()

	t.Run("Empty", func(t *testing.T) {
		messages, err := s.GetMessages(ctx, chatID)
		require.NoError(t, err)
		require.Empty(t, messages)

		unread, err := s.CountUnread(ctx, chatID, account.MustGenerateUserID(), nil)
		require.NoError(t, err)
		require.Zero(t, unread)
	})

	var users []*commonpb.UserId
	for range 2 {
		users = append(users, account.MustGenerateUserID())
	}

	var messages []*messagingpb.Message

	t.Run("Append", func(t *testing.T) {
		for i := range 10 {
			for _, sender := range users {
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
					Ts: timestamppb.Now(),
				}

				// Ensure time ordering is progressing, otherwise ms collisions is
				// non-deterministic (well, won't be post sort, but this way we don't
				// have to sort)
				time.Sleep(time.Millisecond)
				require.NoError(t, s.PutMessage(ctx, chatID, msg))
				require.NotNil(t, msg.MessageId)

				messages = append(messages, msg)
			}
		}
	})

	t.Run("GetMessages", func(t *testing.T) {
		actual, err := s.GetMessages(ctx, chatID)
		require.NoError(t, err)

		require.NoError(t, protoutil.SliceEqualError(messages, actual))
	})

	t.Run("Unread", func(t *testing.T) {
		unread, err := s.CountUnread(ctx, chatID, users[0], nil)
		require.NoError(t, err)
		require.EqualValues(t, 10, unread)

		unread, err = s.CountUnread(ctx, chatID, users[0], messages[10].MessageId)
		require.NoError(t, err)
		require.EqualValues(t, 5, unread)
	})
}
