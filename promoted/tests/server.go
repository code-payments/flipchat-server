package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	promotedpb "github.com/code-payments/flipchat-protobuf-api/generated/go/promoted/v1"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/promoted"
)

func RunServerTests(
	t *testing.T,
	accountStore account.Store,
	chatStore chat.Store,
	promotedStore promoted.Store,
	teardown func(),
) {
	for _, tf := range []func(
		t *testing.T,
		accountStore account.Store,
		chatStore chat.Store,
		promotedStore promoted.Store,
	){
		testServer,
	} {
		tf(t, accountStore, chatStore, promotedStore)
		teardown()
	}
}

func testServer(t *testing.T, accountStore account.Store, chatStore chat.Store, promotedStore promoted.Store) {
	log := zap.Must(zap.NewDevelopment())
	authn := auth.NewKeyPairAuthenticator()
	authz := account.NewAuthorizer(log, accountStore, authn)
	server := promoted.NewServer(log, promotedStore, authz)

	t.Run("No Promoted", func(t *testing.T) {
		ctx := context.Background()
		get, err := server.GetPromotedChats(ctx, &promotedpb.GetPromotedChatsRequest{
			Topic: "tech",
		})
		require.NoError(t, err)
		require.Equal(t, promotedpb.GetPromotedChatsResponse_OK, get.Result)
		require.Nil(t, get.Chats)
	})

	t.Run("Promoted", func(t *testing.T) {
		rooms := createRooms(t, chatStore, 3)

		err := promotedStore.PromoteChat(context.Background(), rooms[0].ChatId, "tech", 50)
		require.NoError(t, err)

		err = promotedStore.PromoteChat(context.Background(), rooms[1].ChatId, "tech", 70)
		require.NoError(t, err)

		err = promotedStore.PromoteChat(context.Background(), rooms[2].ChatId, "health", 30)
		require.NoError(t, err)

		ctx := context.Background()
		get, err := server.GetPromotedChats(ctx, &promotedpb.GetPromotedChatsRequest{
			Topic: "tech",
		})
		require.NoError(t, err)
		require.Equal(t, promotedpb.GetPromotedChatsResponse_OK, get.Result)
		require.Len(t, get.Chats, 2)
		require.Equal(t, rooms[0].ChatId.Value, get.Chats[1].Value)
		require.Equal(t, rooms[1].ChatId.Value, get.Chats[0].Value)
	})

	t.Run("Unknown Topic", func(t *testing.T) {
		ctx := context.Background()
		get, err := server.GetPromotedChats(ctx, &promotedpb.GetPromotedChatsRequest{
			Topic: "rust",
		})
		require.NoError(t, err)
		require.Equal(t, promotedpb.GetPromotedChatsResponse_OK, get.Result)
		require.Nil(t, get.Chats)
	})
}
