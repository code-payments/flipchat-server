package chat

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/query"
)

func TestInMemoryStore_Metadata(t *testing.T) {
	store := NewMemory()

	chatID := model.MustGenerateChatID()
	expected := &chatpb.Metadata{
		ChatId:     chatID,
		Type:       chatpb.Metadata_GROUP,
		Title:      "This is my chat!",
		RoomNumber: 1,
		IsMuted:    false,
		Muteable:   true,
		NumUnread:  0,
	}

	metadata := proto.Clone(expected).(*chatpb.Metadata)
	metadata.RoomNumber = 0
	metadata.IsMuted = true
	metadata.NumUnread = 20

	result, err := store.GetChatMetadata(context.Background(), chatID)
	require.ErrorIs(t, err, ErrChatNotFound)
	require.Nil(t, result)

	created, err := store.CreateChat(context.Background(), metadata)
	require.NoError(t, err)
	require.NoError(t, protoutil.ProtoEqualError(expected, created))

	result, err = store.GetChatMetadata(context.Background(), chatID)
	require.NoError(t, err)
	require.NoError(t, protoutil.ProtoEqualError(expected, result))
}

func TestInMemoryStore_GetAllChatsForUser(t *testing.T) {
	store := NewMemory()

	memberID := account.MustGenerateUserID()

	chatIDs, err := store.GetChatsForUser(context.Background(), memberID)
	require.NoError(t, err)
	require.Empty(t, chatIDs)

	var expectedChatIDs []*commonpb.ChatId
	for i := 0; i < 10; i++ {
		chatID := model.MustGenerateChatID()
		expectedChatIDs = append(expectedChatIDs, chatID)

		_, err = store.CreateChat(context.Background(), &chatpb.Metadata{
			ChatId: chatID,
			Type:   chatpb.Metadata_TWO_WAY,
		})

		for range 2 {
			require.NoError(t, store.AddMember(context.Background(), chatID, Member{
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

func TestInMemoryStore_GetAllChatsForUser_Pagination(t *testing.T) {
	store := NewMemory()

	memberID := account.MustGenerateUserID()

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

		require.NoError(t, store.AddMember(context.Background(), chatID, Member{
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
func TestInMemoryStore_GetChatMembers(t *testing.T) {
	store := NewMemory()

	chatID := model.MustGenerateChatID()
	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)

	var expectedMembers []*Member
	for i := 0; i < 10; i++ {
		member := &Member{
			UserID:  account.MustGenerateUserID(),
			AddedBy: account.MustGenerateUserID(),
			IsMuted: i%2 == 0,
		}

		expectedMembers = append(expectedMembers, member)

		for range 2 {
			require.NoError(t, store.AddMember(context.Background(), chatID, *member))
		}
	}

	slices.SortFunc(expectedMembers, func(a, b *Member) int {
		return bytes.Compare(a.UserID.Value, b.UserID.Value)
	})

	members, err := store.GetMembers(context.Background(), chatID)
	require.NoError(t, err)

	require.Equal(t, len(expectedMembers), len(members))

	for i := range expectedMembers {
		require.NoError(t, protoutil.ProtoEqualError(expectedMembers[i].UserID, members[i].UserID))
		require.NoError(t, protoutil.ProtoEqualError(expectedMembers[i].AddedBy, members[i].AddedBy))
		require.Equal(t, expectedMembers[i].IsMuted, members[i].IsMuted)
	}

}

func TestInMemoryStore_IsChatMember(t *testing.T) {
	store := NewMemory()

	chatID := model.MustGenerateChatID()
	memberID := account.MustGenerateUserID()

	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)

	isMember, err := store.IsMember(context.Background(), chatID, memberID)
	require.NoError(t, err)
	require.False(t, isMember)

	require.NoError(t, store.AddMember(context.Background(), chatID, Member{
		UserID: memberID,
	}))

	isMember, err = store.IsMember(context.Background(), chatID, memberID)
	require.NoError(t, err)
	require.True(t, isMember)
}

func TestInMemoryStore_SetChatMuteState(t *testing.T) {
	store := NewMemory()

	chatID := model.MustGenerateChatID()
	memberID := account.MustGenerateUserID()

	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)

	require.NoError(t, store.AddMember(context.Background(), chatID, Member{
		UserID: memberID,
	}))

	members, err := store.GetMembers(context.Background(), chatID)
	require.NoError(t, err)
	require.False(t, members[0].IsMuted)

	require.NoError(t, store.SetMuteState(context.Background(), chatID, memberID, true))

	members, err = store.GetMembers(context.Background(), chatID)
	require.NoError(t, err)
	require.True(t, members[0].IsMuted)
}

func TestInMemoryStore_JoinLeave(t *testing.T) {
	store := NewMemory()

	chatID := model.MustGenerateChatID()

	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)

	member := Member{
		UserID:  account.MustGenerateUserID(),
		AddedBy: account.MustGenerateUserID(),
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

func TestInMemoryStore_AddRemove(t *testing.T) {
	store := NewMemory()

	chatID := model.MustGenerateChatID()

	_, err := store.CreateChat(context.Background(), &chatpb.Metadata{
		ChatId: chatID,
		Type:   chatpb.Metadata_GROUP,
	})
	require.NoError(t, err)

	var members []*Member
	for range 10 {
		member := Member{
			UserID:  account.MustGenerateUserID(),
			AddedBy: account.MustGenerateUserID(),
		}

		require.NoError(t, store.AddMember(context.Background(), chatID, member))
		members = append(members, &member)
	}

	slices.SortFunc(members, func(a, b *Member) int {
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
