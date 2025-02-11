package tests

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"sort"
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
	profilepb "github.com/code-payments/flipchat-protobuf-api/generated/go/profile/v1"

	codedata "github.com/code-payments/code-server/pkg/code/data"
	codekin "github.com/code-payments/code-server/pkg/kin"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/event"
	"github.com/code-payments/flipchat-server/flags"
	"github.com/code-payments/flipchat-server/intent"
	"github.com/code-payments/flipchat-server/messaging"
	"github.com/code-payments/flipchat-server/model"
	moderation_memory "github.com/code-payments/flipchat-server/moderation/memory"
	"github.com/code-payments/flipchat-server/profile"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/testutil"
)

func RunServerTests(
	t *testing.T,
	accounts account.Store,
	profiles profile.Store,
	chats chat.Store,
	messages messaging.MessageStore,
	pointers messaging.PointerStore,
	intents intent.Store,
	teardown func(),
) {

	for _, tf := range []func(
		t *testing.T,
		accounts account.Store,
		profiles profile.Store,
		chats chat.Store,
		messages messaging.MessageStore,
		pointers messaging.PointerStore,
		intents intent.Store,
	){
		testServer,
	} {
		tf(t, accounts, profiles, chats, messages, pointers, intents)
		teardown()
	}
}

func testServer(
	t *testing.T,
	accounts account.Store,
	profiles profile.Store,
	chats chat.Store,
	messageDB messaging.MessageStore,
	pointerDB messaging.PointerStore,
	intents intent.Store,
) {

	log := zap.Must(zap.NewDevelopment())
	codeData := codedata.NewTestDataProvider()

	userID := model.MustGenerateUserID()
	keyPair := model.MustGenerateKeyPair()
	_, _ = accounts.Bind(context.Background(), userID, keyPair.Proto())
	_ = accounts.SetRegistrationFlag(context.Background(), userID, true)
	_ = profiles.SetDisplayName(context.Background(), userID, "Self")
	bus := event.NewBus[*commonpb.ChatId, *event.ChatEvent](func(id *commonpb.ChatId) []byte {
		return id.Value
	})

	serv := chat.NewServer(
		log,
		account.NewAuthorizer(log, accounts, auth.NewKeyPairAuthenticator()),
		accounts,
		chats,
		intents,
		messageDB,
		pointerDB,
		profiles,
		codeData,
		messaging.NewNoopMessenger(), // todo: add tests for announcements
		moderation_memory.NewClient(false),
		bus,
	)

	cc := testutil.RunGRPCServer(t, testutil.WithService(func(s *grpc.Server) {
		chatpb.RegisterChatServer(s, serv)
	}))

	client := chatpb.NewChatClient(cc)

	verifyExpectedProtoMembers := func(t *testing.T, expected, actual []*chatpb.Member) {
		expectedClone := protoutil.SliceClone(expected)
		actualClone := protoutil.SliceClone(actual)

		sort.Slice(expectedClone, func(i, j int) bool {
			return bytes.Compare(expectedClone[i].UserId.Value, expectedClone[j].UserId.Value) < 0
		})
		sort.Slice(actualClone, func(i, j int) bool {
			return bytes.Compare(actualClone[i].UserId.Value, actualClone[j].UserId.Value) < 0
		})

		if len(expected) != len(actual) {
			fmt.Printf("Expected: %s\nActual: %s\n", expectedClone, actualClone)
		}
		require.Len(t, actualClone, len(expectedClone))

		require.NoError(t, protoutil.SliceEqualError(expectedClone, actualClone))
	}

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
			_ = accounts.SetRegistrationFlag(context.Background(), groupUserID, true)
			require.NoError(t, profiles.SetDisplayName(context.Background(), groupUserID, fmt.Sprintf("User-%d", i)))
			otherUsers = append(otherUsers, groupUserID)
		}

		startPaymentMetadata := &chatpb.StartGroupChatPaymentMetadata{
			UserId: userID,
		}
		startIntentID := testutil.CreatePayment(t, codeData, flags.StartGroupFee, startPaymentMetadata)

		start := &chatpb.StartChatRequest{
			Parameters: &chatpb.StartChatRequest_GroupChat{
				GroupChat: &chatpb.StartChatRequest_StartGroupChatParameters{
					Users:         otherUsers,
					DisplayName:   "Start Group Chat Test",
					PaymentIntent: startIntentID,
				},
			},
		}
		require.NoError(t, keyPair.Auth(start, &start.Auth))

		created, err := client.StartChat(context.Background(), start)
		require.NoError(t, err)
		require.Equal(t, chatpb.StartChatResponse_OK, created.Result)
		require.EqualValues(t, 1, created.Chat.RoomNumber)
		require.Equal(t, start.GetGroupChat().DisplayName, created.Chat.DisplayName)
		require.NoError(t, protoutil.ProtoEqualError(userID, created.Chat.Owner))
		require.Equal(t, chat.InitialMessagingFee, created.Chat.MessagingFee.Quarks)
		require.True(t, created.Chat.OpenStatus.IsCurrentlyOpen)

		expectedMembers := []*chatpb.Member{{
			UserId: userID,
			Identity: &chatpb.MemberIdentity{
				DisplayName: "Self",
			},
			IsSelf:                 true,
			HasModeratorPermission: true,
			HasSendPermission:      true,
		}}

		for i, groupUserID := range otherUsers {
			pointers := []*messagingpb.Pointer{
				{Type: messagingpb.Pointer_READ, Value: messaging.MustGenerateMessageID()},
				{Type: messagingpb.Pointer_DELIVERED, Value: messaging.MustGenerateMessageID()},
			}
			for _, p := range pointers {
				advanced, err := pointerDB.AdvancePointer(context.Background(), created.Chat.ChatId, groupUserID, p)
				require.NoError(t, err)
				require.True(t, advanced)
			}

			expectedMembers = append(expectedMembers, &chatpb.Member{
				UserId: groupUserID,
				Identity: &chatpb.MemberIdentity{
					DisplayName: fmt.Sprintf("User-%d", i),
				},
				Pointers:          pointers,
				IsSelf:            false,
				HasSendPermission: true,
			})
		}

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
		verifyExpectedProtoMembers(t, expectedMembers, get.Members)

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
		verifyExpectedProtoMembers(t, expectedMembers, get.Members)

		getAll := &chatpb.GetChatsRequest{}
		require.NoError(t, keyPair.Auth(getAll, &getAll.Auth))
		getAllResp, err := client.GetChats(context.Background(), getAll)
		require.NoError(t, err)
		require.Equal(t, chatpb.GetChatsResponse_OK, getAllResp.Result)
		require.NoError(t, protoutil.ProtoEqualError(created.Chat, getAllResp.Chats[0]))

		getMemberDelta := &chatpb.GetMemberUpdatesRequest{
			ChatId: created.Chat.ChatId,
		}
		require.NoError(t, keyPair.Auth(getMemberDelta, &getMemberDelta.Auth))
		getMemberDeltaResp, err := client.GetMemberUpdates(context.Background(), getMemberDelta)
		require.NoError(t, err)
		require.Equal(t, chatpb.GetMemberUpdatesResponse_OK, getMemberDeltaResp.Result)
		verifyExpectedProtoMembers(t, expectedMembers, getMemberDeltaResp.Updates[0].GetFullRefresh().Members)
		require.NotNil(t, getMemberDeltaResp.Updates[0].PagingToken)

		t.Run("Leave and join as host", func(t *testing.T) {
			// Leave the room
			leave := &chatpb.LeaveChatRequest{
				ChatId: created.Chat.GetChatId(),
			}
			require.NoError(t, keyPair.Auth(leave, &leave.Auth))

			leaveResp, err := client.LeaveChat(context.Background(), leave)
			require.NoError(t, err)
			require.Equal(t, chatpb.LeaveChatResponse_OK, leaveResp.Result)

			var hostMember *chatpb.Member
			var newExpectedMembers []*chatpb.Member
			for _, member := range expectedMembers {
				if member.HasModeratorPermission {
					hostMember = member
				} else {
					newExpectedMembers = append(newExpectedMembers, member)
				}
			}

			get, err = client.GetChat(context.Background(), getByID)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatResponse_OK, get.Result)
			require.NoError(t, protoutil.ProtoEqualError(created.Chat, get.Metadata))
			verifyExpectedProtoMembers(t, newExpectedMembers, get.Members)

			// Join the room without payment but with full permissions
			join := &chatpb.JoinChatRequest{
				Identifier: &chatpb.JoinChatRequest_ChatId{
					ChatId: created.Chat.GetChatId(),
				},
			}
			require.NoError(t, keyPair.Auth(join, &join.Auth))

			newExpectedMembers = append(newExpectedMembers, hostMember)

			joinResp, err := client.JoinChat(context.Background(), join)
			require.NoError(t, err)
			require.Equal(t, chatpb.JoinChatResponse_OK, joinResp.Result)
			require.NoError(t, protoutil.ProtoEqualError(created.Chat, joinResp.Metadata))
			verifyExpectedProtoMembers(t, newExpectedMembers, joinResp.Members)
		})

		t.Run("Join and leave", func(t *testing.T) {
			otherUser := model.MustGenerateUserID()
			otherKeyPair := model.MustGenerateKeyPair()
			_, _ = accounts.Bind(context.Background(), otherUser, otherKeyPair.Proto())
			_ = accounts.SetRegistrationFlag(context.Background(), otherUser, true)

			newExpectedMember := &chatpb.Member{
				UserId:            otherUser,
				Identity:          &chatpb.MemberIdentity{},
				IsSelf:            true,
				HasSendPermission: false,
			}
			newExpectedMembers := protoutil.SliceClone(expectedMembers)
			for _, m := range newExpectedMembers {
				m.IsSelf = false
			}
			newExpectedMembers = append(newExpectedMembers, newExpectedMember)

			join := &chatpb.JoinChatRequest{
				Identifier: &chatpb.JoinChatRequest_ChatId{
					ChatId: created.Chat.GetChatId(),
				},
				WithoutSendPermission: true,
			}
			require.NoError(t, otherKeyPair.Auth(join, &join.Auth))

			joinResp, err := client.JoinChat(context.Background(), join)
			require.NoError(t, err)
			require.Equal(t, chatpb.JoinChatResponse_OK, joinResp.Result)
			require.NoError(t, protoutil.ProtoEqualError(created.Chat, joinResp.Metadata))
			verifyExpectedProtoMembers(t, newExpectedMembers, joinResp.Members)

			leave := &chatpb.LeaveChatRequest{
				ChatId: created.Chat.GetChatId(),
			}
			require.NoError(t, otherKeyPair.Auth(leave, &leave.Auth))

			leaveResp, err := client.LeaveChat(context.Background(), leave)
			require.NoError(t, err)
			require.Equal(t, chatpb.LeaveChatResponse_OK, leaveResp.Result)

			newExpectedMembers = newExpectedMembers[:len(newExpectedMembers)-1]

			getByID := &chatpb.GetChatRequest{
				Identifier: &chatpb.GetChatRequest_ChatId{
					ChatId: created.Chat.GetChatId(),
				},
			}
			require.NoError(t, otherKeyPair.Auth(getByID, &getByID.Auth))
			get, err = client.GetChat(context.Background(), getByID)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatResponse_OK, get.Result)
			require.NoError(t, protoutil.ProtoEqualError(created.Chat, get.Metadata))
			verifyExpectedProtoMembers(t, newExpectedMembers, get.Members)

			newExpectedMembers = append(newExpectedMembers, newExpectedMember)

			join = &chatpb.JoinChatRequest{
				Identifier: &chatpb.JoinChatRequest_RoomId{
					RoomId: created.Chat.RoomNumber,
				},
				WithoutSendPermission: true,
			}
			require.NoError(t, otherKeyPair.Auth(join, &join.Auth))
			require.Equal(t, chatpb.JoinChatResponse_OK, joinResp.Result)
			require.NoError(t, protoutil.ProtoEqualError(created.Chat, joinResp.Metadata))
			verifyExpectedProtoMembers(t, newExpectedMembers, joinResp.Members)
		})

		t.Run("Promote and demote user", func(t *testing.T) {
			otherUser := model.MustGenerateUserID()
			otherKeyPair := model.MustGenerateKeyPair()
			_, _ = accounts.Bind(context.Background(), otherUser, otherKeyPair.Proto())
			_ = accounts.SetRegistrationFlag(context.Background(), otherUser, true)

			newExpectedMembers := protoutil.SliceClone(expectedMembers)
			for _, m := range newExpectedMembers {
				m.IsSelf = false
			}
			newExpectedMembers = append(newExpectedMembers, &chatpb.Member{
				UserId:            otherUser,
				Identity:          &chatpb.MemberIdentity{},
				IsSelf:            true,
				HasSendPermission: false,
			})

			// Join without send permission
			join := &chatpb.JoinChatRequest{
				Identifier: &chatpb.JoinChatRequest_ChatId{
					ChatId: created.Chat.GetChatId(),
				},
				WithoutSendPermission: true,
			}
			require.NoError(t, otherKeyPair.Auth(join, &join.Auth))
			joinResp, err := client.JoinChat(context.Background(), join)
			require.NoError(t, err)
			require.Equal(t, chatpb.JoinChatResponse_OK, joinResp.Result)
			require.NoError(t, protoutil.ProtoEqualError(created.Chat, joinResp.Metadata))
			verifyExpectedProtoMembers(t, newExpectedMembers, joinResp.Members)

			promote := &chatpb.PromoteUserRequest{
				ChatId:               created.Chat.ChatId,
				UserId:               otherUser,
				EnableSendPermission: true,
			}
			require.NoError(t, keyPair.Auth(promote, &promote.Auth))
			promoteResp, err := client.PromoteUser(context.Background(), promote)
			require.NoError(t, err)
			require.Equal(t, chatpb.PromoteUserResponse_OK, promoteResp.Result)

			newExpectedMembers[len(newExpectedMembers)-1].HasSendPermission = true
			getByID := &chatpb.GetChatRequest{
				Identifier: &chatpb.GetChatRequest_ChatId{
					ChatId: created.Chat.GetChatId(),
				},
			}
			require.NoError(t, otherKeyPair.Auth(getByID, &getByID.Auth))
			get, err := client.GetChat(context.Background(), getByID)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatResponse_OK, get.Result)
			verifyExpectedProtoMembers(t, newExpectedMembers, get.Members)

			demote := &chatpb.DemoteUserRequest{
				ChatId:                created.Chat.ChatId,
				UserId:                otherUser,
				DisableSendPermission: true,
			}
			require.NoError(t, keyPair.Auth(demote, &demote.Auth))
			demoteResp, err := client.DemoteUser(context.Background(), demote)
			require.NoError(t, err)
			require.Equal(t, chatpb.DemoteUserResponse_OK, demoteResp.Result)

			newExpectedMembers[len(newExpectedMembers)-1].HasSendPermission = false
			get, err = client.GetChat(context.Background(), getByID)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatResponse_OK, get.Result)
			require.NoError(t, protoutil.ProtoEqualError(created.Chat, get.Metadata))
			verifyExpectedProtoMembers(t, newExpectedMembers, get.Members)

			leave := &chatpb.LeaveChatRequest{
				ChatId: created.Chat.GetChatId(),
			}
			require.NoError(t, otherKeyPair.Auth(leave, &leave.Auth))
			leaveResp, err := client.LeaveChat(context.Background(), leave)
			require.NoError(t, err)
			require.Equal(t, chatpb.LeaveChatResponse_OK, leaveResp.Result)
		})

		t.Run("Remove user", func(t *testing.T) {
			t.Skip("feature disabled")

			removedUser := otherUsers[0]
			expectedMembers = expectedMembers[1:]

			remove := &chatpb.RemoveUserRequest{
				ChatId: created.Chat.GetChatId(),
				UserId: otherUsers[0],
			}
			require.NoError(t, keyPair.Auth(remove, &remove.Auth))

			removeResp, err := client.RemoveUser(context.Background(), remove)
			require.NoError(t, err)
			require.Equal(t, chatpb.RemoveUserResponse_OK, removeResp.Result)

			get, err = client.GetChat(context.Background(), getByID)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatResponse_OK, get.Result)
			require.Len(t, get.Members, len(expectedMembers))
			for _, member := range get.Members {
				require.NotEqual(t, removedUser.Value, member.UserId.Value)
			}
		})

		t.Run("Permanently mute user", func(t *testing.T) {
			otherUser := model.MustGenerateUserID()
			otherKeyPair := model.MustGenerateKeyPair()
			_, _ = accounts.Bind(context.Background(), otherUser, otherKeyPair.Proto())
			_ = accounts.SetRegistrationFlag(context.Background(), otherUser, true)

			join := &chatpb.JoinChatRequest{
				Identifier:            &chatpb.JoinChatRequest_ChatId{ChatId: created.Chat.ChatId},
				WithoutSendPermission: true,
			}
			require.NoError(t, otherKeyPair.Auth(join, &join.Auth))
			joinResp, err := client.JoinChat(context.Background(), join)
			require.NoError(t, err)
			require.Equal(t, chatpb.JoinChatResponse_OK, joinResp.Result)

			mute := &chatpb.MuteUserRequest{
				ChatId: created.Chat.ChatId,
				UserId: otherUser,
			}
			require.NoError(t, keyPair.Auth(mute, &mute.Auth))
			muteResp, err := client.MuteUser(context.Background(), mute)
			require.NoError(t, err)
			require.Equal(t, chatpb.MuteUserResponse_OK, muteResp.Result)

			leave := &chatpb.LeaveChatRequest{
				ChatId: created.Chat.ChatId,
			}
			require.NoError(t, otherKeyPair.Auth(leave, &leave.Auth))
			leaveResp, err := client.LeaveChat(context.Background(), leave)
			require.NoError(t, err)
			require.Equal(t, chatpb.LeaveChatResponse_OK, leaveResp.Result)

			joinResp, err = client.JoinChat(context.Background(), join)
			require.NoError(t, err)
			require.Equal(t, chatpb.JoinChatResponse_OK, joinResp.Result)

			newExpectedMembers := protoutil.SliceClone(expectedMembers)
			for _, m := range newExpectedMembers {
				m.IsSelf = false
			}
			newExpectedMembers = append(newExpectedMembers, &chatpb.Member{
				UserId:   otherUser,
				Identity: &chatpb.MemberIdentity{},
				IsSelf:   true,
				IsMuted:  true,
			})

			getByID := &chatpb.GetChatRequest{
				Identifier: &chatpb.GetChatRequest_ChatId{
					ChatId: created.Chat.GetChatId(),
				},
			}
			require.NoError(t, otherKeyPair.Auth(getByID, &getByID.Auth))
			get, err := client.GetChat(context.Background(), getByID)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatResponse_OK, get.Result)
			require.Len(t, get.Members, len(newExpectedMembers))
			verifyExpectedProtoMembers(t, newExpectedMembers, get.Members)

			leaveResp, err = client.LeaveChat(context.Background(), leave)
			require.NoError(t, err)
			require.Equal(t, chatpb.LeaveChatResponse_OK, leaveResp.Result)
		})

		t.Run("Set messaging fee", func(t *testing.T) {
			setMessagingFee := &chatpb.SetMessagingFeeRequest{
				ChatId:       created.Chat.ChatId,
				MessagingFee: &commonpb.PaymentAmount{Quarks: codekin.ToQuarks(500)},
			}
			require.NoError(t, keyPair.Auth(setMessagingFee, &setMessagingFee.Auth))

			setMessagingFeeResp, err := client.SetMessagingFee(context.Background(), setMessagingFee)
			require.NoError(t, err)
			require.Equal(t, chatpb.SetMessagingFeeResponse_OK, setMessagingFeeResp.Result)

			get, err := client.GetChat(context.Background(), getByID)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatResponse_OK, get.Result)
			require.NoError(t, protoutil.ProtoEqualError(setMessagingFee.MessagingFee, get.Metadata.MessagingFee))
		})

		t.Run("Set display name", func(t *testing.T) {
			setDisplayName := &chatpb.SetDisplayNameRequest{
				ChatId:      created.Chat.ChatId,
				DisplayName: "My Room",
			}
			require.NoError(t, keyPair.Auth(setDisplayName, &setDisplayName.Auth))

			setDisplayNameResp, err := client.SetDisplayName(context.Background(), setDisplayName)
			require.NoError(t, err)
			require.Equal(t, chatpb.SetDisplayNameResponse_OK, setDisplayNameResp.Result)

			get, err := client.GetChat(context.Background(), getByID)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatResponse_OK, get.Result)
			require.Equal(t, setDisplayName.DisplayName, get.Metadata.DisplayName)
		})

		t.Run("Remove display name", func(t *testing.T) {
			setDisplayName := &chatpb.SetDisplayNameRequest{
				ChatId:      created.Chat.ChatId,
				DisplayName: "",
			}
			require.NoError(t, keyPair.Auth(setDisplayName, &setDisplayName.Auth))

			setDisplayNameResp, err := client.SetDisplayName(context.Background(), setDisplayName)
			require.NoError(t, err)
			require.Equal(t, chatpb.SetDisplayNameResponse_OK, setDisplayNameResp.Result)

			get, err := client.GetChat(context.Background(), getByID)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatResponse_OK, get.Result)
			require.Equal(t, setDisplayName.DisplayName, get.Metadata.DisplayName)
		})

		t.Run("Close and open room", func(t *testing.T) {
			close := &chatpb.CloseChatRequest{
				ChatId: created.Chat.ChatId,
			}
			require.NoError(t, keyPair.Auth(close, &close.Auth))

			closeResp, err := client.CloseChat(context.Background(), close)
			require.NoError(t, err)
			require.Equal(t, chatpb.CloseChatResponse_OK, closeResp.Result)

			get, err := client.GetChat(context.Background(), getByID)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatResponse_OK, get.Result)
			require.False(t, get.Metadata.OpenStatus.IsCurrentlyOpen)

			open := &chatpb.OpenChatRequest{
				ChatId: created.Chat.ChatId,
			}
			require.NoError(t, keyPair.Auth(open, &open.Auth))

			openResp, err := client.OpenChat(context.Background(), open)
			require.NoError(t, err)
			require.Equal(t, chatpb.OpenChatResponse_OK, openResp.Result)

			get, err = client.GetChat(context.Background(), getByID)
			require.NoError(t, err)
			require.Equal(t, chatpb.GetChatResponse_OK, get.Result)
			require.True(t, get.Metadata.OpenStatus.IsCurrentlyOpen)
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

	// todo: add RemoveUser test when feature is enabled
	t.Run("Stream Events", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		streamUser := model.MustGenerateUserID()
		streamKeyPair := model.MustGenerateKeyPair()
		_, _ = accounts.Bind(ctx, streamUser, streamKeyPair.Proto())
		_ = accounts.SetRegistrationFlag(context.Background(), streamUser, true)

		streamUserProfile := &profilepb.UserProfile{DisplayName: "Stream User"}
		require.NoError(t, profiles.SetDisplayName(context.Background(), streamUser, streamUserProfile.DisplayName))

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

		// To avoid races with flush
		time.Sleep(200 * time.Millisecond)

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

		verifyExpectedFullMemberRefresh := func(update *chatpb.MemberUpdate, expected []chat.Member) {
			refresh := update.GetFullRefresh()
			require.NotNil(t, refresh)
			require.Len(t, refresh.Members, len(expected))

			slices.SortFunc(refresh.Members, func(a, b *chatpb.Member) int {
				return bytes.Compare(a.UserId.Value, b.UserId.Value)
			})
			slices.SortFunc(expected, func(a, b chat.Member) int {
				return bytes.Compare(a.UserID.Value, b.UserID.Value)
			})

			for i := range expected {
				require.NoError(t, protoutil.ProtoEqualError(expected[i].UserID, refresh.Members[i].UserId))
			}
		}

		startPaymentMetadata := &chatpb.StartGroupChatPaymentMetadata{
			UserId: streamUser,
		}
		startIntentID := testutil.CreatePayment(t, codeData, flags.StartGroupFee, startPaymentMetadata)
		start := &chatpb.StartChatRequest{
			Parameters: &chatpb.StartChatRequest_GroupChat{
				GroupChat: &chatpb.StartChatRequest_StartGroupChatParameters{
					Users:         []*commonpb.UserId{userID},
					DisplayName:   "Stream Test",
					PaymentIntent: startIntentID,
				},
			},
		}
		require.NoError(t, streamKeyPair.Auth(start, &start.Auth))
		started, err := client.StartChat(ctx, start)
		require.NoError(t, err)

		u := <-updateCh
		require.NoError(t, protoutil.ProtoEqualError(started.Chat.ChatId, u.ChatId))
		require.Len(t, u.MetadataUpdates, 1)
		require.Len(t, u.MemberUpdates, 1)
		require.NoError(t, protoutil.ProtoEqualError(started.Chat, u.MetadataUpdates[0].GetFullRefresh().Metadata))
		verifyExpectedFullMemberRefresh(
			u.MemberUpdates[0],
			[]chat.Member{
				{UserID: streamUser, AddedBy: streamUser, IsMuted: false, HasModPermission: true},
				{UserID: userID, AddedBy: streamUser, IsMuted: false, HasModPermission: false},
			},
		)

		// other user leaves chat (notification on leave)
		leave := &chatpb.LeaveChatRequest{ChatId: started.Chat.ChatId}
		require.NoError(t, keyPair.Auth(leave, &leave.Auth))
		_, _ = client.LeaveChat(context.Background(), leave)

		u = <-updateCh
		require.NoError(t, protoutil.ProtoEqualError(started.Chat.ChatId, u.ChatId))
		require.Empty(t, u.MetadataUpdates)
		require.Len(t, u.MemberUpdates, 1)
		require.NoError(t, protoutil.ProtoEqualError(u.MemberUpdates[0].GetLeft().Member, userID))

		// Other user creates a group (which we will join)
		startPaymentMetadata = &chatpb.StartGroupChatPaymentMetadata{
			UserId: userID,
		}
		start.GetGroupChat().PaymentIntent = testutil.CreatePayment(t, codeData, flags.StartGroupFee, startPaymentMetadata)
		require.NoError(t, keyPair.Auth(start, &start.Auth))
		startedOther, err := client.StartChat(ctx, start)
		require.NoError(t, err)

		join := &chatpb.JoinChatRequest{
			Identifier:            &chatpb.JoinChatRequest_ChatId{ChatId: startedOther.Chat.ChatId},
			WithoutSendPermission: true,
		}
		require.NoError(t, streamKeyPair.Auth(join, &join.Auth))

		joined, err := client.JoinChat(ctx, join)
		require.NoError(t, err)

		u = <-updateCh
		require.NoError(t, protoutil.ProtoEqualError(joined.Metadata.ChatId, u.ChatId))
		require.Empty(t, u.MetadataUpdates)
		require.Len(t, u.MemberUpdates, 1)
		require.NoError(t, protoutil.ProtoEqualError(u.MemberUpdates[0].GetJoined().Member.UserId, streamUser))
		require.Equal(t, streamUserProfile.DisplayName, u.MemberUpdates[0].GetJoined().Member.Identity.DisplayName)

		// Other user closes the chat
		close := &chatpb.CloseChatRequest{
			ChatId: startedOther.Chat.ChatId,
		}
		require.NoError(t, keyPair.Auth(close, &close.Auth))
		_, err = client.CloseChat(context.Background(), close)
		require.NoError(t, err)

		u = <-updateCh
		require.NoError(t, protoutil.ProtoEqualError(joined.Metadata.ChatId, u.ChatId))
		require.Len(t, u.MetadataUpdates, 1)
		require.Empty(t, u.MemberUpdates)
		require.False(t, u.MetadataUpdates[0].GetOpenStatusChanged().NewOpenStatus.IsCurrentlyOpen)

		// Other user opens the chat
		open := &chatpb.OpenChatRequest{
			ChatId: startedOther.Chat.ChatId,
		}
		require.NoError(t, keyPair.Auth(open, &open.Auth))
		_, err = client.OpenChat(context.Background(), open)
		require.NoError(t, err)

		u = <-updateCh
		require.NoError(t, protoutil.ProtoEqualError(joined.Metadata.ChatId, u.ChatId))
		require.Len(t, u.MetadataUpdates, 1)
		require.Empty(t, u.MemberUpdates)
		require.True(t, u.MetadataUpdates[0].GetOpenStatusChanged().NewOpenStatus.IsCurrentlyOpen)

		// Other user updates messaging fee
		setMessagingFee := &chatpb.SetMessagingFeeRequest{
			ChatId: startedOther.Chat.ChatId,
			MessagingFee: &commonpb.PaymentAmount{
				Quarks: 2 * chat.InitialMessagingFee,
			},
		}
		require.NoError(t, keyPair.Auth(setMessagingFee, &setMessagingFee.Auth))
		_, err = client.SetMessagingFee(ctx, setMessagingFee)
		require.NoError(t, err)

		u = <-updateCh
		require.NoError(t, protoutil.ProtoEqualError(joined.Metadata.ChatId, u.ChatId))
		require.Len(t, u.MetadataUpdates, 1)
		require.Empty(t, u.MemberUpdates, 0)
		require.NoError(t, protoutil.ProtoEqualError(u.MetadataUpdates[0].GetMessagingFeeChanged().NewMessagingFee, setMessagingFee.MessagingFee))

		// Other user updates chat display name
		setDisplayName := &chatpb.SetDisplayNameRequest{
			ChatId:      startedOther.Chat.ChatId,
			DisplayName: "My Room",
		}
		require.NoError(t, keyPair.Auth(setDisplayName, &setDisplayName.Auth))
		_, err = client.SetDisplayName(ctx, setDisplayName)
		require.NoError(t, err)

		u = <-updateCh
		require.NoError(t, protoutil.ProtoEqualError(joined.Metadata.ChatId, u.ChatId))
		require.Len(t, u.MetadataUpdates, 1)
		require.Empty(t, u.MemberUpdates, 0)
		require.Equal(t, u.MetadataUpdates[0].GetDisplayNameChanged().NewDisplayName, setDisplayName.DisplayName)

		// Other user sends messages in the chat
		//
		// todo: proper integration test with messenger
		for numMessagesSent := 1; numMessagesSent < 2*int(chat.MaxUnreadCount); numMessagesSent++ {
			expectedNumUnread := uint32(numMessagesSent)
			expectedHasMoreUnread := false
			if numMessagesSent > int(chat.MaxUnreadCount) {
				expectedNumUnread = chat.MaxUnreadCount
				expectedHasMoreUnread = true
			}
			chatMsg := &messagingpb.Message{
				SenderId: userID,
				Content: []*messagingpb.Content{
					{
						Type: &messagingpb.Content_Text{Text: &messagingpb.TextContent{Text: fmt.Sprintf("msg%d", numMessagesSent)}},
					},
				},
			}
			chatMsg, err = messageDB.PutMessage(ctx, startedOther.Chat.ChatId, chatMsg)
			require.NoError(t, err)
			require.NoError(t, bus.OnEvent(startedOther.Chat.ChatId, &event.ChatEvent{ChatID: startedOther.Chat.ChatId, MessageUpdate: chatMsg}))
			u = <-updateCh
			require.NoError(t, protoutil.ProtoEqualError(joined.Metadata.ChatId, u.ChatId))
			require.Len(t, u.MetadataUpdates, 2)
			require.Empty(t, u.MemberUpdates)
			require.NotNil(t, u.LastMessage)
			require.NoError(t, protoutil.ProtoEqualError(chatMsg.Ts, u.MetadataUpdates[0].GetLastActivityChanged().NewLastActivity))
			require.NoError(t, protoutil.ProtoEqualError(&chatpb.MetadataUpdate_UnreadCountChanged{NumUnread: expectedNumUnread, HasMoreUnread: expectedHasMoreUnread}, u.MetadataUpdates[1].GetUnreadCountChanged()))
			require.NoError(t, protoutil.ProtoEqualError(chatMsg, u.LastMessage))
		}

		// Other user mutes us
		mute := &chatpb.MuteUserRequest{
			ChatId: startedOther.Chat.ChatId,
			UserId: streamUser,
		}
		require.NoError(t, keyPair.Auth(mute, &mute.Auth))
		_, err = client.MuteUser(ctx, mute)
		require.NoError(t, err)

		u = <-updateCh
		require.NoError(t, protoutil.ProtoEqualError(joined.Metadata.ChatId, u.ChatId))
		require.Empty(t, u.MetadataUpdates)
		require.Len(t, u.MemberUpdates, 1)
		require.NoError(t, protoutil.ProtoEqualError(u.MemberUpdates[0].GetMuted().Member, streamUser))
		require.NoError(t, protoutil.ProtoEqualError(u.MemberUpdates[0].GetMuted().MutedBy, userID))

		// Other user promotes us
		promote := &chatpb.PromoteUserRequest{
			ChatId:               startedOther.Chat.ChatId,
			UserId:               streamUser,
			EnableSendPermission: true,
		}
		require.NoError(t, keyPair.Auth(promote, &promote.Auth))
		_, err = client.PromoteUser(context.Background(), promote)
		require.NoError(t, err)

		u = <-updateCh
		require.NoError(t, protoutil.ProtoEqualError(joined.Metadata.ChatId, u.ChatId))
		require.Empty(t, u.MetadataUpdates)
		require.Len(t, u.MemberUpdates, 1)
		require.NoError(t, protoutil.ProtoEqualError(u.MemberUpdates[0].GetPromoted().Member, streamUser))
		require.NoError(t, protoutil.ProtoEqualError(u.MemberUpdates[0].GetPromoted().PromotedBy, userID))
		require.True(t, u.MemberUpdates[0].GetPromoted().SendPermissionEnabled)

		// Other user demotes us
		demote := &chatpb.DemoteUserRequest{
			ChatId:                startedOther.Chat.ChatId,
			UserId:                streamUser,
			DisableSendPermission: true,
		}
		require.NoError(t, keyPair.Auth(demote, &demote.Auth))
		_, err = client.DemoteUser(context.Background(), demote)
		require.NoError(t, err)

		u = <-updateCh
		require.NoError(t, protoutil.ProtoEqualError(joined.Metadata.ChatId, u.ChatId))
		require.Empty(t, u.MetadataUpdates)
		require.Len(t, u.MemberUpdates, 1)
		require.NoError(t, protoutil.ProtoEqualError(u.MemberUpdates[0].GetDemoted().Member, streamUser))
		require.NoError(t, protoutil.ProtoEqualError(u.MemberUpdates[0].GetDemoted().DemotedBy, userID))
		require.True(t, u.MemberUpdates[0].GetDemoted().SendPermissionDisabled)

		// Leave the chat
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
		_ = accounts.SetRegistrationFlag(context.Background(), streamUser, true)

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
