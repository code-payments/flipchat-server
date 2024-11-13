package chat

import (
	"bytes"
	"context"
	"fmt"
	"slices"
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
		require.NoError(t, protoutil.ProtoEqualError(userID, created.Chat.Owner))

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

	t.Run("Stream Events", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		streamUser := model.MustGenerateUserID()
		streamKeyPair := model.MustGenerateKeyPair()
		_, _ = accounts.Bind(ctx, streamUser, streamKeyPair.Proto())

		stream, err := client.StreamChatEvents(ctx)
		require.NoError(t, err)

		req := &chatpb.StreamChatEventsRequest{
			Type: &chatpb.StreamChatEventsRequest_Params_{
				Params: &chatpb.StreamChatEventsRequest_Params{
					Ts: timestamppb.Now(),
				},
			},
		}
		require.NoError(t, streamKeyPair.Auth(req.GetParams(), &req.GetParams().Auth))
		require.NoError(t, stream.Send(req))

		updateCh := make(chan *chatpb.StreamChatEventsResponse_ChatUpdate, 1024)

		go func() {
			for {
				resp, err := stream.Recv()
				if codes.Canceled == status.Code(err) {
					return
				}
				require.NoError(t, err)

				switch typed := resp.Type.(type) {
				case *chatpb.StreamChatEventsResponse_Ping:
					_ = stream.Send(&chatpb.StreamChatEventsRequest{
						Type: &chatpb.StreamChatEventsRequest_Pong{
							Pong: &commonpb.ClientPong{Timestamp: timestamppb.Now()},
						},
					})
				case *chatpb.StreamChatEventsResponse_Events:
					for _, u := range typed.Events.Updates {
						updateCh <- u
					}
				}
			}
		}()

		// TODO: There's a bit of a race for 'flush initial state', so we just wait a bit
		time.Sleep(200 * time.Millisecond)

		verifyExpectedMembers := func(update *chatpb.StreamChatEventsResponse_MemberUpdate, expected []Member) {
			refresh := update.GetRefresh()
			require.NotNil(t, refresh)

			slices.SortFunc(refresh.Members, func(a, b *chatpb.Member) int {
				return bytes.Compare(a.UserId.Value, b.UserId.Value)
			})
			slices.SortFunc(expected, func(a, b Member) int {
				return bytes.Compare(a.UserID.Value, b.UserID.Value)
			})
		}

		start := &chatpb.StartChatRequest{
			Parameters: &chatpb.StartChatRequest_GroupChat{
				GroupChat: &chatpb.StartChatRequest_StartGroupChatParameters{
					Title: "my-title",
					Users: []*commonpb.UserId{userID},
				},
			},
		}
		require.NoError(t, streamKeyPair.Auth(start, &start.Auth))
		started, err := client.StartChat(ctx, start)
		require.NoError(t, err)

		u := <-updateCh
		require.NoError(t, protoutil.ProtoEqualError(started.Chat.ChatId, u.ChatId))
		require.NoError(t, protoutil.ProtoEqualError(started.Chat, u.Metadata))
		verifyExpectedMembers(
			u.MemberUpdate,
			[]Member{
				{UserID: streamUser, AddedBy: streamUser, IsMuted: false, IsHost: true},
				{UserID: userID, AddedBy: streamUser, IsMuted: false, IsHost: false},
			},
		)

		// other user leaves chat (notification on leave)
		leave := &chatpb.LeaveChatRequest{ChatId: started.Chat.ChatId}
		require.NoError(t, keyPair.Auth(leave, &leave.Auth))
		_, _ = client.LeaveChat(context.Background(), leave)

		u = <-updateCh
		require.Nil(t, u.Metadata)
		require.NoError(t, protoutil.ProtoEqualError(started.Chat.ChatId, u.ChatId))
		verifyExpectedMembers(
			u.MemberUpdate,
			[]Member{
				{UserID: streamUser, AddedBy: streamUser, IsMuted: false, IsHost: true},
			},
		)

		// Other user creates a group (which we will join)
		require.NoError(t, keyPair.Auth(start, &start.Auth))
		startedOther, err := client.StartChat(ctx, start)
		require.NoError(t, err)

		join := &chatpb.JoinChatRequest{
			Identifier:    &chatpb.JoinChatRequest_ChatId{ChatId: startedOther.Chat.ChatId},
			PaymentIntent: model.MustGenerateIntentID(),
		}
		require.NoError(t, streamKeyPair.Auth(join, &join.Auth))

		joined, err := client.JoinChat(ctx, join)
		require.NoError(t, err)

		u = <-updateCh
		require.NoError(t, protoutil.ProtoEqualError(joined.Metadata, u.Metadata))
		require.NoError(t, protoutil.ProtoEqualError(joined.Metadata.ChatId, u.ChatId))
		verifyExpectedMembers(
			u.MemberUpdate,
			[]Member{
				{UserID: streamUser, AddedBy: streamUser, IsMuted: false, IsHost: false},
				{UserID: userID, AddedBy: streamUser, IsMuted: false, IsHost: true},
			},
		)

		leave = &chatpb.LeaveChatRequest{ChatId: started.Chat.ChatId}
		require.NoError(t, streamKeyPair.Auth(leave, &leave.Auth))
		_, _ = client.LeaveChat(context.Background(), leave)

		select {
		case <-updateCh:
			require.Fail(t, "should not have produced an update")
		case <-time.After(time.Second):
		}
	})

	t.Run("Duplicate Streams", func(t *testing.T) {
		streamA, err := client.StreamChatEvents(context.Background())
		require.NoError(t, err)

		streamUser := model.MustGenerateUserID()
		streamKeyPair := model.MustGenerateKeyPair()
		_, _ = accounts.Bind(context.Background(), streamUser, streamKeyPair.Proto())

		params := &chatpb.StreamChatEventsRequest_Params{
			Ts: timestamppb.Now(),
		}
		require.NoError(t, streamKeyPair.Auth(params, &params.Auth))
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
