package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/promoted"
)

func RunStoreTests(
	t *testing.T,
	chatStore chat.Store,
	promotedStore promoted.Store,
	teardown func(),
) {

	for _, tf := range []func(
		t *testing.T,
		chatStore chat.Store,
		promotedStore promoted.Store,
	){
		testPromotedStore_Success,
		testPromotedStore_Multiple,
		testPromotedStore_UnknownTopic,
		testPromotedStore_ScoreUpdate,
		testPromotedStore_Demote,
	} {
		tf(t, chatStore, promotedStore)
		teardown()
	}
}

func testPromotedStore_Success(t *testing.T, chatStore chat.Store, promotedStore promoted.Store) {
	ctx := context.Background()

	rooms := createRooms(t, chatStore, 3)

	err := promotedStore.PromoteChat(ctx, rooms[0].ChatId, "golang", 100)
	require.NoError(t, err)

	chats, err := promotedStore.GetPromotedChats(ctx, "golang")
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.Equal(t, rooms[0].ChatId, chats[0].ChatID)
	require.Equal(t, 100, chats[0].Score)
	require.Equal(t, "golang", chats[0].Topic)
}

func testPromotedStore_Multiple(t *testing.T, chatStore chat.Store, promotedStore promoted.Store) {
	ctx := context.Background()

	rooms := createRooms(t, chatStore, 3)

	err := promotedStore.PromoteChat(ctx, rooms[0].ChatId, "golang", 100)
	require.NoError(t, err)

	err = promotedStore.PromoteChat(ctx, rooms[1].ChatId, "golang", 200)
	require.NoError(t, err)

	err = promotedStore.PromoteChat(ctx, rooms[2].ChatId, "python", 80)
	require.NoError(t, err)

	chats, err := promotedStore.GetPromotedChats(ctx, "golang")
	require.NoError(t, err)
	require.Len(t, chats, 2)
	require.Equal(t, rooms[0].ChatId.Value, chats[1].ChatID.Value)
	require.Equal(t, 100, chats[1].Score)
	require.Equal(t, rooms[1].ChatId.Value, chats[0].ChatID.Value)
	require.Equal(t, 200, chats[0].Score)

	chats, err = promotedStore.GetPromotedChats(ctx, "python")
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.Equal(t, rooms[2].ChatId.Value, chats[0].ChatID.Value)
}

func testPromotedStore_UnknownTopic(t *testing.T, chatStore chat.Store, promotedStore promoted.Store) {
	ctx := context.Background()

	chats, err := promotedStore.GetPromotedChats(ctx, "rust")
	require.NoError(t, err)
	require.Len(t, chats, 0)
}

func testPromotedStore_ScoreUpdate(t *testing.T, chatStore chat.Store, promotedStore promoted.Store) {
	ctx := context.Background()

	rooms := createRooms(t, chatStore, 3)

	err := promotedStore.PromoteChat(ctx, rooms[0].ChatId, "golang", 100)
	require.NoError(t, err)

	err = promotedStore.PromoteChat(ctx, rooms[0].ChatId, "golang", 200)
	require.NoError(t, err)

	chats, err := promotedStore.GetPromotedChats(ctx, "golang")
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.Equal(t, rooms[0].ChatId.Value, chats[0].ChatID.Value)
	require.Equal(t, 200, chats[0].Score)
}

func testPromotedStore_Demote(t *testing.T, chatStore chat.Store, promotedStore promoted.Store) {
	ctx := context.Background()

	rooms := createRooms(t, chatStore, 3)

	err := promotedStore.PromoteChat(ctx, rooms[0].ChatId, "golang", 100)
	require.NoError(t, err)

	err = promotedStore.PromoteChat(ctx, rooms[1].ChatId, "golang", 200)
	require.NoError(t, err)

	err = promotedStore.PromoteChat(ctx, rooms[2].ChatId, "python", 80)
	require.NoError(t, err)

	err = promotedStore.DemoteChat(ctx, rooms[1].ChatId, "golang")
	require.NoError(t, err)

	chats, err := promotedStore.GetPromotedChats(ctx, "golang")
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.Equal(t, rooms[0].ChatId.Value, chats[0].ChatID.Value)

	chats, err = promotedStore.GetPromotedChats(ctx, "python")
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.Equal(t, rooms[2].ChatId.Value, chats[0].ChatID.Value)
}
