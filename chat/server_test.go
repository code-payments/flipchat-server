package chat

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
	"github.com/code-payments/flipchat-server/event"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/messaging"
	"github.com/code-payments/flipchat-server/profile"
	"github.com/code-payments/flipchat-server/testutil"
)

func TestServer(t *testing.T) {
	log := zap.Must(zap.NewDevelopment())
	accounts := account.NewInMemory()
	messageDB := messaging.NewMemory()
	profiles := profile.NewInMemory()

	userID := model.MustGenerateUserID()
	keyPair := model.MustGenerateKeyPair()
	_, _ = accounts.Bind(context.Background(), userID, keyPair.Proto())
	_ = profiles.SetDisplayName(context.Background(), userID, "Self")
	bus := event.NewBus[*commonpb.ChatId, *event.ChatEvent](func(id *commonpb.ChatId) []byte {
		return id.Value
	})

	serv := NewServer(
		log,
		account.NewAuthorizer(log, accounts, auth.NewKeyPairAuthenticator()),
		NewMemory(),
		profiles,
		messageDB,
		messageDB,
		bus,
	)

	cc := testutil.RunGRPCServer(t, testutil.WithService(func(s *grpc.Server) {
		chatpb.RegisterChatServer(s, serv)
	}))

	client := chatpb.NewChatClient(cc)

	t.Run("Empty", func(t *testing.T) {
		chatID := model.MustGenerateChatID()

		getChat := &chatpb.GetChatRequest{
			Identifier: &chatpb.GetChatRequest_ChatId{
				ChatId: chatID,
			},
		}
		require.NoError(t, keyPair.Auth(getChat, &getChat.Auth))
		getChatResp, err := client.GetChat(context.Background(), getChat)
		require.NoError(t, err)
		require.Equal(t, chatpb.GetChatResponse_NOT_FOUND, getChatResp.Result)

		getRoom := &chatpb.GetChatRequest{
			Identifier: &chatpb.GetChatRequest_RoomNumber{
				RoomNumber: 1,
			},
		}
		require.NoError(t, keyPair.Auth(getRoom, &getRoom.Auth))
		getRoomResp, err := client.GetChat(context.Background(), getChat)
		require.NoError(t, err)
		require.Equal(t, chatpb.GetChatResponse_NOT_FOUND, getRoomResp.Result)

		getAll := &chatpb.GetChatsRequest{}
		require.NoError(t, keyPair.Auth(getAll, &getAll.Auth))
		getAllResp, err := client.GetChats(context.Background(), getAll)
		require.NoError(t, err)
		require.Equal(t, chatpb.GetChatsResponse_OK, getAllResp.Result)
		require.Empty(t, getAllResp.Chats)
	})

	t.Run("Start Group", func(t *testing.T) {
		var otherUsers []*commonpb.UserId
		for i := range 5 {
			groupUserID := model.MustGenerateUserID()
			_, _ = accounts.Bind(context.Background(), groupUserID, model.MustGenerateKeyPair().Proto())
			require.NoError(t, profiles.SetDisplayName(context.Background(), groupUserID, fmt.Sprintf("User-%d", i)))
			otherUsers = append(otherUsers, groupUserID)
		}

		start := &chatpb.StartChatRequest{
			Parameters: &chatpb.StartChatRequest_GroupChat{
				GroupChat: &chatpb.StartChatRequest_StartGroupChatParameters{
					Users: otherUsers,
					Title: "My Fun Group!",
				},
			},
		}
		require.NoError(t, keyPair.Auth(start, &start.Auth))

		created, err := client.StartChat(context.Background(), start)
		require.NoError(t, err)
		require.Equal(t, chatpb.StartChatResponse_OK, created.Result)
		require.Equal(t, "My Fun Group!", created.Chat.Title)
		require.EqualValues(t, 1, created.Chat.RoomNumber)

		expectedMembers := []*chatpb.Member{{
			UserId: userID,
			Identity: &chatpb.MemberIdentity{
				DisplayName: "Self",
			},
			IsSelf: true,
			IsHost: true,
		}}

		for i, groupUserID := range otherUsers {
			pointers := []*messagingpb.Pointer{
				{Type: messagingpb.Pointer_READ, Value: messaging.MustGenerateMessageID()},
				{Type: messagingpb.Pointer_DELIVERED, Value: messaging.MustGenerateMessageID()},
			}
			for _, p := range pointers {
				advanced, err := messageDB.AdvancePointer(context.Background(), created.Chat.ChatId, groupUserID, p)
				require.NoError(t, err)
				require.True(t, advanced)
			}

			expectedMembers = append(expectedMembers, &chatpb.Member{
				UserId: groupUserID,
				Identity: &chatpb.MemberIdentity{
					DisplayName: fmt.Sprintf("User-%d", i),
				},
				Pointers: pointers,
				IsSelf:   false,
			})
		}

		slices.SortFunc(expectedMembers, func(a, b *chatpb.Member) int {
			return bytes.Compare(a.UserId.Value, b.UserId.Value)
		})

		getByID := &chatpb.GetChatRequest{
			Identifier: &chatpb.GetChatRequest_ChatId{
				ChatId: created.Chat.GetChatId(),
			},
		}
		require.NoError(t, keyPair.Auth(getByID, &getByID.Auth))

		get, err := client.GetChat(context.Background(), getByID)
		require.NoError(t, err)
		require.Equal(t, chatpb.GetChatResponse_OK, get.Result)
		require.NoError(t, protoutil.ProtoEqualError(created.Chat, get.Metadata))
		require.NoError(t, protoutil.SliceEqualError(expectedMembers, get.Members))

		getByRoom := &chatpb.GetChatRequest{
			Identifier: &chatpb.GetChatRequest_RoomNumber{
				RoomNumber: created.Chat.GetRoomNumber(),
			},
		}
		require.NoError(t, keyPair.Auth(getByRoom, &getByRoom.Auth))
		get, err = client.GetChat(context.Background(), getByID)
		require.NoError(t, err)
		require.Equal(t, chatpb.GetChatResponse_OK, get.Result)
		require.NoError(t, protoutil.ProtoEqualError(created.Chat, get.Metadata))
		require.NoError(t, protoutil.SliceEqualError(expectedMembers, get.Members))

		getAll := &chatpb.GetChatsRequest{}
		require.NoError(t, keyPair.Auth(getAll, &getAll.Auth))
		getAllResp, err := client.GetChats(context.Background(), getAll)
		require.NoError(t, err)
		require.Equal(t, chatpb.GetChatsResponse_OK, getAllResp.Result)
		require.NoError(t, protoutil.ProtoEqualError(created.Chat, getAllResp.Chats[0]))

		t.Run("Join and leave", func(t *testing.T) {
			otherUser := model.MustGenerateUserID()
			otherKeyPair := model.MustGenerateKeyPair()
			_, _ = accounts.Bind(context.Background(), otherUser, otherKeyPair.Proto())

			newExpectedMembers := protoutil.SliceClone(expectedMembers)
			for _, m := range newExpectedMembers {
				m.IsSelf = false
			}
			newExpectedMembers = append(newExpectedMembers, &chatpb.Member{
				UserId:   otherUser,
				Identity: &chatpb.MemberIdentity{},
				IsSelf:   true,
			})

			slices.SortFunc(newExpectedMembers, func(a, b *chatpb.Member) int {
				return bytes.Compare(a.UserId.Value, b.UserId.Value)
			})

			join := &chatpb.JoinChatRequest{
				Identifier: &chatpb.JoinChatRequest_ChatId{
					ChatId: created.Chat.GetChatId(),
				},
				PaymentIntent: model.MustGenerateIntentID(),
			}
			require.NoError(t, otherKeyPair.Auth(join, &join.Auth))

			joinResp, err := client.JoinChat(context.Background(), join)
			require.NoError(t, err)
			require.Equal(t, chatpb.JoinChatResponse_OK, joinResp.Result)
			require.NoError(t, protoutil.ProtoEqualError(created.Chat, joinResp.Metadata))
			require.NoError(t, protoutil.SliceEqualError(newExpectedMembers, joinResp.Members))

			leave := &chatpb.LeaveChatRequest{
				ChatId: created.Chat.GetChatId(),
			}
			require.NoError(t, otherKeyPair.Auth(leave, &leave.Auth))

			leaveResp, err := client.LeaveChat(context.Background(), leave)
			require.NoError(t, err)
			require.Equal(t, chatpb.LeaveChatResponse_OK, leaveResp.Result)

			get, err = client.GetChat(context.Background(), getByID)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatResponse_OK, get.Result)
			require.NoError(t, protoutil.ProtoEqualError(created.Chat, get.Metadata))
			require.NoError(t, protoutil.SliceEqualError(expectedMembers, get.Members))

			join = &chatpb.JoinChatRequest{
				Identifier: &chatpb.JoinChatRequest_RoomId{
					RoomId: created.Chat.RoomNumber,
				},
				PaymentIntent: model.MustGenerateIntentID(),
			}
			require.NoError(t, otherKeyPair.Auth(join, &join.Auth))
			require.Equal(t, chatpb.JoinChatResponse_OK, joinResp.Result)
			require.NoError(t, protoutil.ProtoEqualError(created.Chat, joinResp.Metadata))
			require.NoError(t, protoutil.SliceEqualError(newExpectedMembers, joinResp.Members))
		})
	})

	t.Run("Start Two Way", func(t *testing.T) {
		otherUserID := model.MustGenerateUserID()
		otherKeyPair := model.MustGenerateKeyPair()

		start := &chatpb.StartChatRequest{
			Parameters: &chatpb.StartChatRequest_TwoWayChat{
				TwoWayChat: &chatpb.StartChatRequest_StartTwoWayChatParameters{
					OtherUserId: otherUserID,
				},
			},
		}
		require.NoError(t, keyPair.Auth(start, &start.Auth))

		original, err := client.StartChat(context.Background(), start)
		require.NoError(t, err)
		require.Equal(t, chatpb.StartChatResponse_OK, original.Result)

		// Two-way chats should always resolve to the same chat, and
		// therefor is idempotent (even during reversal of users).

		created, err := client.StartChat(context.Background(), start)
		require.NoError(t, err)
		require.Equal(t, chatpb.StartChatResponse_OK, created.Result)
		require.NoError(t, protoutil.ProtoEqualError(original, created))

		startOther := &chatpb.StartChatRequest{
			Parameters: &chatpb.StartChatRequest_TwoWayChat{
				TwoWayChat: &chatpb.StartChatRequest_StartTwoWayChatParameters{
					OtherUserId: userID,
				},
			},
		}
		require.NoError(t, otherKeyPair.Auth(startOther, &startOther.Auth))

		created, err = client.StartChat(context.Background(), start)
		require.NoError(t, err)
		require.Equal(t, chatpb.StartChatResponse_OK, created.Result)
		require.NoError(t, protoutil.ProtoEqualError(original, created))
	})

	t.Run("All Chats", func(t *testing.T) {
		get := &chatpb.GetChatsRequest{}
		require.NoError(t, keyPair.Auth(get, &get.Auth))
		resp, err := client.GetChats(context.Background(), get)
		require.NoError(t, err)
		require.Len(t, resp.GetChats(), 2)
	})

	t.Run("Duplicate Streams", func(t *testing.T) {
		streamA, err := client.StreamChatEvents(context.Background())
		require.NoError(t, err)

		params := &chatpb.StreamChatEventsRequest_Params{}
		require.NoError(t, keyPair.Auth(params, &params.Auth))
		err = streamA.Send(&chatpb.StreamChatEventsRequest{Type: &chatpb.StreamChatEventsRequest_Params_{Params: params}})
		require.NoError(t, err)
		_, _ = streamA.Recv() // Ping

		streamB, err := client.StreamChatEvents(context.Background())
		require.NoError(t, err)
		err = streamB.Send(&chatpb.StreamChatEventsRequest{Type: &chatpb.StreamChatEventsRequest_Params_{Params: params}})
		require.NoError(t, err)
		_, _ = streamB.Recv() // Ping

		_, err = streamA.Recv()
		require.Equal(t, codes.Aborted, status.Code(err))
	})
}
