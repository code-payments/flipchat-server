package tests

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/code-payments/code-server/pkg/kin"
	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/query"
)

func RunStoreTests(t *testing.T, s chat.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s chat.Store){
		testChatStore_Metadata,
		testChatStore_GetAllChatsForUser,
		testChatStore_GetAllChatsForUser_Pagination,
		testChatStore_GetChatMembers,
		testChatStore_IsChatMember,
		testChatStore_SetChatMuteState,
		testChatStore_SetChatPushState,
		testChatStore_JoinLeave,
		testChatStore_JoinLeaveWithPermissions,
		testChatStore_AddRemove,
		testChatStore_SetDisplayName,
		testChatStore_SetCoverCharge,
		testChatStore_AdvanceLastChatActivity,
	} {
		tf(t, s)
		teardown()
	}
}

func testChatStore_Metadata(t *testing.T, store chat.Store) {
	chatID1 := model.MustGenerateChatID()
	expected1 := &chatpb.Metadata{
		ChatId:       chatID1,
		Type:         chatpb.Metadata_GROUP,
		Owner:        model.MustGenerateUserID(),
		CoverCharge:  &commonpb.PaymentAmount{Quarks: 1},
		RoomNumber:   1,
		NumUnread:    0,
		LastActivity: &timestamppb.Timestamp{Seconds: time.Now().Unix()},
	}

	chatID2 := model.MustGenerateChatID()
	expected2 := &chatpb.Metadata{
		ChatId:       chatID2,
		Type:         chatpb.Metadata_GROUP,
		Owner:        model.MustGenerateUserID(),
		CoverCharge:  &commonpb.PaymentAmount{Quarks: 2},
		RoomNumber:   2,
		NumUnread:    0,
		LastActivity: &timestamppb.Timestamp{Seconds: time.Now().Unix()},
	}

	metadata1 := proto.Clone(expected1).(*chatpb.Metadata)
	metadata1.RoomNumber = 0
	metadata1.IsPushEnabled = true
	metadata1.CanDisablePush = true
	metadata1.NumUnread = 14
	metadata1.HasMoreUnread = true

	metadata2 := proto.Clone(expected2).(*chatpb.Metadata)
	metadata2.RoomNumber = 0
	metadata2.IsPushEnabled = true
	metadata2.CanDisablePush = true
	metadata2.NumUnread = 42
	metadata2.HasMoreUnread = true

	result, err := store.GetChatMetadata(context.Background(), chatID1)
	require.ErrorIs(t, err, chat.ErrChatNotFound)
	require.Nil(t, result)

	created, err := store.CreateChat(context.Background(), metadata1)
	require.NoError(t, err)
	require.NoError(t, protoutil.ProtoEqualError(expected1, created))

	result, err = store.GetChatMetadata(context.Background(), chatID1)
	require.NoError(t, err)
	require.NoError(t, protoutil.ProtoEqualError(expected1, result))

	created, err = store.CreateChat(context.Background(), metadata2)
	require.NoError(t, err)
	require.NoError(t, protoutil.ProtoEqualError(expected2, created))

	result, err = store.GetChatMetadata(context.Background(), chatID2)
	require.NoError(t, err)
	require.NoError(t, protoutil.ProtoEqualError(expected2, result))

	_, err = store.GetChatMetadataBatched(context.Background(), model.MustGenerateChatID())
	require.Equal(t, chat.ErrChatNotFound, err)

	_, err = store.GetChatMetadataBatched(context.Background(), chatID1, chatID2, model.MustGenerateChatID())
	require.Equal(t, chat.ErrChatNotFound, err)

	results, err := store.GetChatMetadataBatched(context.Background(), chatID1, chatID2)
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.NoError(t, protoutil.ProtoEqualError(expected1, results[0]))
	require.NoError(t, protoutil.ProtoEqualError(expected2, results[1]))
}

func testChatStore_GetAllChatsForUser(t *testing.T, store chat.Store) {

	memberID := model.MustGenerateUserID()

	chatIDs, err := store.GetChatsForUser(context.Background(), memberID)
	require.NoError(t, err)
	require.Empty(t, chatIDs)

	var expectedChatIDs []*commonpb.ChatId
	for i := 0; i < 10; i++ {
		chatID := model.MustGenerateChatID()
		expectedChatIDs = append(expectedChatIDs, chatID)

		_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
			ChatId: chatID,
			Type:   chatpb.Metadata_TWO_WAY,
		})

		require.NoError(t, err)

		for range 2 {
			require.NoError(t, store.AddMember(context.Background(), chatID, chat.Member{
				UserID: memberID,
			}))
		}
	}

	slices.SortFunc(expectedChatIDs, func(a, b *commonpb.ChatId) int {
		return bytes.Compare(a.Value, b.Value)
	})

	chatIDs, err = store.GetChatsForUser(context.Background(), memberID)
	require.NoError(t, err)
	require.NoError(t, protoutil.SliceEqualError(expectedChatIDs, chatIDs))
}

func testChatStore_GetAllChatsForUser_Pagination(t *testing.T, store chat.Store) {

	memberID := model.MustGenerateUserID()

	// Create 10 chats
	var chatIDs []*commonpb.ChatId
	for i := 0; i < 10; i++ {
		chatID := model.MustGenerateChatID()
		chatIDs = append(chatIDs, chatID)

		_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
			ChatId: chatID,
			Type:   chatpb.Metadata_TWO_WAY,
		})
		require.NoError(t, err)

		require.NoError(t, store.AddMember(context.Background(), chatID, chat.Member{
			UserID: memberID,
		}))
	}

	slices.SortFunc(chatIDs, func(a, b *commonpb.ChatId) int {
		return bytes.Compare(a.Value, b.Value)
	})

	reversedChatIds := slices.Clone(chatIDs)
	slices.Reverse(reversedChatIds)

	t.Run("Ascending Order", func(t *testing.T) {
		result, err := store.GetChatsForUser(context.Background(), memberID, query.WithAscending())
		require.NoError(t, err)
		require.Equal(t, chatIDs, result)
	})

	t.Run("Descending Order", func(t *testing.T) {
		result, err := store.GetChatsForUser(context.Background(), memberID, query.WithDescending())
		require.NoError(t, err)
		require.Equal(t, reversedChatIds, result)
	})

	t.Run("With Cursor", func(t *testing.T) {
		cursor := &commonpb.PagingToken{Value: chatIDs[3].Value}
		result, err := store.GetChatsForUser(context.Background(), memberID, query.WithAscending(), query.WithToken(cursor))
		require.NoError(t, err)
		require.Equal(t, chatIDs[4:], result)
	})

	t.Run("With Cursor (Descending)", func(t *testing.T) {
		cursor := &commonpb.PagingToken{Value: reversedChatIds[6].Value}
		result, err := store.GetChatsForUser(context.Background(), memberID, query.WithDescending(), query.WithToken(cursor))
		require.NoError(t, err)
		require.Equal(t, reversedChatIds[7:], result)
	})

	t.Run("With Limit", func(t *testing.T) {
		result, err := store.GetChatsForUser(context.Background(), memberID, query.WithLimit(5))
		require.NoError(t, err)
		require.Equal(t, chatIDs[:5], result)
	})

	t.Run("With Limit (Descending)", func(t *testing.T) {
		cursor := &commonpb.PagingToken{Value: reversedChatIds[4].Value}
		result, err := store.GetChatsForUser(context.Background(), memberID, query.WithDescending(), query.WithToken(cursor), query.WithLimit(3))
		require.NoError(t, err)
		require.Equal(t, reversedChatIds[5:8], result)
	})
}

// TODO: Need proper pagination tests
func testChatStore_GetChatMembers(t *testing.T, store chat.Store) {

	chatID := model.MustGenerateChatID()
	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)

	var expectedMembers []*chat.Member
	for i := 0; i < 10; i++ {
		member := &chat.Member{
			UserID:  model.MustGenerateUserID(),
			AddedBy: model.MustGenerateUserID(),
			IsMuted: i%2 == 0,
		}

		expectedMembers = append(expectedMembers, member)

		for range 2 {
			require.NoError(t, store.AddMember(context.Background(), chatID, *member))
		}
	}

	slices.SortFunc(expectedMembers, func(a, b *chat.Member) int {
		return bytes.Compare(a.UserID.Value, b.UserID.Value)
	})

	members, err := store.GetMembers(context.Background(), chatID)
	require.NoError(t, err)
	require.Equal(t, len(expectedMembers), len(members))

	for i := range expectedMembers {
		require.NoError(t, protoutil.ProtoEqualError(expectedMembers[i].UserID, members[i].UserID))
		require.NoError(t, protoutil.ProtoEqualError(expectedMembers[i].AddedBy, members[i].AddedBy))
		require.Equal(t, expectedMembers[i].IsMuted, members[i].IsMuted)

		// Expect push to be enabled by default
		require.True(t, members[i].IsPushEnabled)
	}

}

func testChatStore_IsChatMember(t *testing.T, store chat.Store) {

	chatID := model.MustGenerateChatID()
	memberID := model.MustGenerateUserID()

	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)

	isMember, err := store.IsMember(context.Background(), chatID, memberID)
	require.NoError(t, err)
	require.False(t, isMember)

	require.NoError(t, store.AddMember(context.Background(), chatID, chat.Member{
		UserID: memberID,
	}))

	isMember, err = store.IsMember(context.Background(), chatID, memberID)
	require.NoError(t, err)
	require.True(t, isMember)
}

func testChatStore_SetChatMuteState(t *testing.T, store chat.Store) {

	chatID := model.MustGenerateChatID()
	memberID := model.MustGenerateUserID()

	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)

	require.NoError(t, store.AddMember(context.Background(), chatID, chat.Member{
		UserID: memberID,
	}))

	members, err := store.GetMembers(context.Background(), chatID)
	require.NoError(t, err)
	require.False(t, members[0].IsMuted)

	hasMuteState, err := store.IsUserMuted(context.Background(), chatID, memberID)
	require.NoError(t, err)
	require.False(t, hasMuteState)

	require.NoError(t, store.SetMuteState(context.Background(), chatID, memberID, true))

	members, err = store.GetMembers(context.Background(), chatID)
	require.NoError(t, err)
	require.True(t, members[0].IsMuted)

	hasMuteState, err = store.IsUserMuted(context.Background(), chatID, memberID)
	require.NoError(t, err)
	require.True(t, hasMuteState)
}

func testChatStore_SetSendPermission(t *testing.T, store chat.Store) {

	chatID := model.MustGenerateChatID()
	memberID := model.MustGenerateUserID()

	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)

	require.NoError(t, store.AddMember(context.Background(), chatID, chat.Member{
		UserID: memberID,
	}))

	members, err := store.GetMembers(context.Background(), chatID)
	require.NoError(t, err)
	require.False(t, members[0].HasSendPermission) // Default to false

	hasSendPermission, err := store.HasSendPermission(context.Background(), chatID, memberID)
	require.NoError(t, err)
	require.False(t, hasSendPermission)

	require.NoError(t, store.SetSendPermission(context.Background(), chatID, memberID, true))

	members, err = store.GetMembers(context.Background(), chatID)
	require.NoError(t, err)
	require.True(t, members[0].HasSendPermission)

	hasSendPermission, err = store.HasSendPermission(context.Background(), chatID, memberID)
	require.NoError(t, err)
	require.True(t, hasSendPermission)
}

func testChatStore_SetChatPushState(t *testing.T, store chat.Store) {
	chatID := model.MustGenerateChatID()
	memberID := model.MustGenerateUserID()

	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)

	require.NoError(t, store.AddMember(context.Background(), chatID, chat.Member{
		UserID: memberID,
	}))

	members, err := store.GetMembers(context.Background(), chatID)
	require.NoError(t, err)
	require.True(t, members[0].IsPushEnabled) // Default to true

	hasPushEnabled, err := store.IsPushEnabled(context.Background(), chatID, memberID)
	require.NoError(t, err)
	require.True(t, hasPushEnabled)

	require.NoError(t, store.SetPushState(context.Background(), chatID, memberID, false))

	members, err = store.GetMembers(context.Background(), chatID)
	require.NoError(t, err)
	require.False(t, members[0].IsPushEnabled)

	hasPushEnabled, err = store.IsPushEnabled(context.Background(), chatID, memberID)
	require.NoError(t, err)
	require.False(t, hasPushEnabled)
}

func testChatStore_JoinLeave(t *testing.T, store chat.Store) {

	chatID := model.MustGenerateChatID()

	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)

	member := chat.Member{
		UserID:  model.MustGenerateUserID(),
		AddedBy: model.MustGenerateUserID(),
	}

	require.NoError(t, store.AddMember(context.Background(), chatID, member))

	chats, err := store.GetChatsForUser(context.Background(), member.UserID)
	require.NoError(t, err)
	require.Len(t, chats, 1)

	require.NoError(t, store.RemoveMember(context.Background(), chatID, member.UserID))

	chats, err = store.GetChatsForUser(context.Background(), member.UserID)
	require.NoError(t, err)
	require.Empty(t, chats, 0)
}

func testChatStore_JoinLeaveWithPermissions(t *testing.T, store chat.Store) {
	chatID := model.MustGenerateChatID()

	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)

	var members []*chat.Member
	for i := range 10 {
		member := chat.Member{
			UserID:  model.MustGenerateUserID(),
			AddedBy: model.MustGenerateUserID(),

			HasModPermission:  i%2 == 0,
			HasSendPermission: i%3 == 0,
		}

		require.NoError(t, store.AddMember(context.Background(), chatID, member))
		members = append(members, &member)
	}

	slices.SortFunc(members, func(a, b *chat.Member) int {
		return bytes.Compare(a.UserID.Value, b.UserID.Value)
	})

	actual, err := store.GetMembers(context.Background(), chatID)
	require.NoError(t, err)
	require.Equal(t, len(members), len(actual))
	for i := range members {
		require.NoError(t, protoutil.ProtoEqualError(members[i].UserID, actual[i].UserID))
		require.NoError(t, protoutil.ProtoEqualError(members[i].AddedBy, actual[i].AddedBy))

		require.Equal(t, members[i].HasModPermission, actual[i].HasModPermission)
		require.Equal(t, members[i].HasSendPermission, actual[i].HasSendPermission)
	}

	require.NoError(t, store.RemoveMember(context.Background(), chatID, members[5].UserID))
	require.NoError(t, store.RemoveMember(context.Background(), chatID, members[5].UserID))
	members = slices.Delete(members, 5, 6)

	actual, err = store.GetMembers(context.Background(), chatID)
	require.NoError(t, err)
	require.Equal(t, len(members), len(actual))
	for i := range members {
		require.NoError(t, protoutil.ProtoEqualError(members[i].UserID, actual[i].UserID))
		require.NoError(t, protoutil.ProtoEqualError(members[i].AddedBy, actual[i].AddedBy))

		require.Equal(t, members[i].HasModPermission, actual[i].HasModPermission)
		require.Equal(t, members[i].HasSendPermission, actual[i].HasSendPermission)
	}
}

func testChatStore_AddRemove(t *testing.T, store chat.Store) {

	chatID := model.MustGenerateChatID()

	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)

	var members []*chat.Member
	for range 10 {
		member := chat.Member{
			UserID:  model.MustGenerateUserID(),
			AddedBy: model.MustGenerateUserID(),
		}

		require.NoError(t, store.AddMember(context.Background(), chatID, member))
		members = append(members, &member)
	}

	slices.SortFunc(members, func(a, b *chat.Member) int {
		return bytes.Compare(a.UserID.Value, b.UserID.Value)
	})

	actual, err := store.GetMembers(context.Background(), chatID)
	require.NoError(t, err)
	require.Equal(t, len(members), len(actual))
	for i := range members {
		require.NoError(t, protoutil.ProtoEqualError(members[i].UserID, actual[i].UserID))
		require.NoError(t, protoutil.ProtoEqualError(members[i].AddedBy, actual[i].AddedBy))
	}

	require.NoError(t, store.RemoveMember(context.Background(), chatID, members[5].UserID))
	require.NoError(t, store.RemoveMember(context.Background(), chatID, members[5].UserID))
	members = slices.Delete(members, 5, 6)

	actual, err = store.GetMembers(context.Background(), chatID)
	require.NoError(t, err)
	require.Equal(t, len(members), len(actual))
	for i := range members {
		require.NoError(t, protoutil.ProtoEqualError(members[i].UserID, actual[i].UserID))
		require.NoError(t, protoutil.ProtoEqualError(members[i].AddedBy, actual[i].AddedBy))
	}
}

func testChatStore_SetDisplayName(t *testing.T, store chat.Store) {
	chatID := model.MustGenerateChatID()

	require.Equal(t, chat.ErrChatNotFound, store.SetDisplayName(context.Background(), chatID, "My Room"))

	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId:       chatID,
		Type:         chatpb.Metadata_GROUP,
		LastActivity: &timestamppb.Timestamp{Seconds: time.Now().Unix()},
	})
	require.NoError(t, err)

	result, err := store.GetChatMetadata(context.Background(), chatID)
	require.NoError(t, err)
	require.Empty(t, result.DisplayName)

	require.NoError(t, store.SetDisplayName(context.Background(), chatID, "My Room"))

	result, err = store.GetChatMetadata(context.Background(), chatID)
	require.NoError(t, err)
	require.Equal(t, "My Room", result.DisplayName)

	require.NoError(t, store.SetDisplayName(context.Background(), chatID, ""))

	result, err = store.GetChatMetadata(context.Background(), chatID)
	require.NoError(t, err)
	require.Equal(t, "", result.DisplayName)
}

func testChatStore_SetCoverCharge(t *testing.T, store chat.Store) {
	chatID := model.MustGenerateChatID()

	require.Equal(t, chat.ErrChatNotFound, store.SetCoverCharge(context.Background(), chatID, &commonpb.PaymentAmount{Quarks: kin.ToQuarks(100)}))

	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId:       chatID,
		Type:         chatpb.Metadata_GROUP,
		LastActivity: &timestamppb.Timestamp{Seconds: time.Now().Unix()},
	})
	require.NoError(t, err)

	result, err := store.GetChatMetadata(context.Background(), chatID)
	require.NoError(t, err)
	require.Nil(t, result.CoverCharge)

	require.NoError(t, store.SetCoverCharge(context.Background(), chatID, &commonpb.PaymentAmount{Quarks: kin.ToQuarks(100)}))

	result, err = store.GetChatMetadata(context.Background(), chatID)
	require.NoError(t, err)
	require.Equal(t, kin.ToQuarks(100), result.CoverCharge.Quarks)
}

func testChatStore_AdvanceLastChatActivity(t *testing.T, store chat.Store) {
	chatID := model.MustGenerateChatID()

	ts := time.Now().Add(30 * time.Second)

	require.ErrorIs(t, chat.ErrChatNotFound, store.AdvanceLastChatActivity(context.Background(), chatID, ts))

	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)

	require.NoError(t, store.AdvanceLastChatActivity(context.Background(), chatID, ts))

	result, err := store.GetChatMetadata(context.Background(), chatID)
	require.NoError(t, err)
	require.Equal(t, ts.Unix(), result.LastActivity.Seconds)
}
