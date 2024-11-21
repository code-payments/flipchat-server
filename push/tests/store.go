package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	pushpb "github.com/code-payments/flipchat-protobuf-api/generated/go/push/v1"

	"github.com/code-payments/flipchat-server/push"
)

func RunStoreTests(t *testing.T, s push.TokenStore, teardown func()) {
	for _, tf := range []func(t *testing.T, s push.TokenStore){
		testAddAndGetTokens,
		testUpdateExistingToken,
		testDeleteToken,
		testClearToken,
		testMultipleUsers,
	} {
		tf(t, s)
		teardown()
	}
}

func testAddAndGetTokens(t *testing.T, store push.TokenStore) {
	ctx := context.Background()

	userID := &commonpb.UserId{Value: []byte("user1")}
	appInstallID1 := &commonpb.AppInstallId{Value: "device1"}
	appInstallID2 := &commonpb.AppInstallId{Value: "device2"}

	// Initially no tokens
	tokens, err := store.GetTokens(ctx, userID)
	require.NoError(t, err)
	assert.Empty(t, tokens)

	// Add tokens for two devices
	err = store.AddToken(ctx, userID, appInstallID1, pushpb.TokenType_FCM_APNS, "token1")
	require.NoError(t, err)

	err = store.AddToken(ctx, userID, appInstallID2, pushpb.TokenType_FCM_APNS, "token2")
	require.NoError(t, err)

	// Verify both tokens are retrieved
	tokens, err = store.GetTokens(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, tokens, 2)

	// Verify token contents
	tokenMap := make(map[string]push.Token)
	for _, token := range tokens {
		tokenMap[token.AppInstallID] = token
	}

	assert.Equal(t, "token1", tokenMap[appInstallID1.Value].Token)
	assert.Equal(t, "token2", tokenMap[appInstallID2.Value].Token)
}

func testUpdateExistingToken(t *testing.T, store push.TokenStore) {
	ctx := context.Background()
	userID := &commonpb.UserId{Value: []byte("user1")}
	appInstallID := &commonpb.AppInstallId{Value: "device1"}

	// Add initial token
	err := store.AddToken(ctx, userID, appInstallID, pushpb.TokenType_FCM_APNS, "token1")
	require.NoError(t, err)

	// Update token
	err = store.AddToken(ctx, userID, appInstallID, pushpb.TokenType_FCM_APNS, "token2")
	require.NoError(t, err)

	// Verify updated token
	tokens, err := store.GetTokens(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, tokens, 1)
	assert.Equal(t, "token2", tokens[0].Token)
}

func testDeleteToken(t *testing.T, store push.TokenStore) {
	ctx := context.Background()

	userID := &commonpb.UserId{Value: []byte("user1")}
	appInstallID := &commonpb.AppInstallId{Value: "device1"}

	// Add token
	err := store.AddToken(ctx, userID, appInstallID, pushpb.TokenType_FCM_APNS, "token1")
	require.NoError(t, err)

	// Delete token
	err = store.DeleteToken(ctx, pushpb.TokenType_FCM_APNS, "token1")
	require.NoError(t, err)

	// Verify token is deleted
	tokens, err := store.GetTokens(ctx, userID)
	require.NoError(t, err)
	assert.Empty(t, tokens)
}

func testClearToken(t *testing.T, store push.TokenStore) {
	ctx := context.Background()

	userID := &commonpb.UserId{Value: []byte("user1")}
	appInstallID1 := &commonpb.AppInstallId{Value: "device1"}
	appInstallID2 := &commonpb.AppInstallId{Value: "device2"}

	// Add tokens for two devices
	err := store.AddToken(ctx, userID, appInstallID1, pushpb.TokenType_FCM_APNS, "token1")
	require.NoError(t, err)

	err = store.AddToken(ctx, userID, appInstallID2, pushpb.TokenType_FCM_APNS, "token2")
	require.NoError(t, err)

	// Clear all tokens
	err = store.ClearTokens(ctx, userID)
	require.NoError(t, err)

	// Verify all tokens are cleared
	tokens, err := store.GetTokens(ctx, userID)
	require.NoError(t, err)
	assert.Empty(t, tokens)
}

func testMultipleUsers(t *testing.T, store push.TokenStore) {
	ctx := context.Background()

	user1 := &commonpb.UserId{Value: []byte("user1")}
	user2 := &commonpb.UserId{Value: []byte("user2")}
	appInstallID := &commonpb.AppInstallId{Value: "device1"}

	// Add tokens for both users
	err := store.AddToken(ctx, user1, appInstallID, pushpb.TokenType_FCM_APNS, "token1")
	require.NoError(t, err)

	err = store.AddToken(ctx, user2, appInstallID, pushpb.TokenType_FCM_APNS, "token2")
	require.NoError(t, err)

	// Verify user1's tokens
	tokens, err := store.GetTokens(ctx, user1)
	require.NoError(t, err)
	assert.Len(t, tokens, 1)
	assert.Equal(t, "token1", tokens[0].Token)

	// Verify user2's tokens
	tokens, err = store.GetTokens(ctx, user2)
	require.NoError(t, err)
	assert.Len(t, tokens, 1)
	assert.Equal(t, "token2", tokens[0].Token)
}
