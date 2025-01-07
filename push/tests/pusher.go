package tests

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	"firebase.google.com/go/v4/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	pushpb "github.com/code-payments/flipchat-protobuf-api/generated/go/push/v1"

	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/push"
)

// testFCMClient captures the messages sent for verification
type testFCMClient struct {
	sentMessage *messaging.MulticastMessage
}

func (c *testFCMClient) SendEachForMulticast(_ context.Context, message *messaging.MulticastMessage) (*messaging.BatchResponse, error) {
	c.sentMessage = message
	return &messaging.BatchResponse{
		SuccessCount: len(message.Tokens),
		Responses:    make([]*messaging.SendResponse, len(message.Tokens)),
	}, nil
}

func RunPusherTests(t *testing.T, s push.TokenStore, teardown func()) {
	for _, tf := range []func(t *testing.T, s push.TokenStore){
		testFCMPusher_SendPush,
	} {
		tf(t, s)
		teardown()
	}
}

func testFCMPusher_SendPush(t *testing.T, store push.TokenStore) {
	ctx := context.Background()

	fcmClient := &testFCMClient{}
	pusher := push.NewFCMPusher(zap.NewNop(), store, fcmClient)

	// Create 5 users with 2 tokens each
	users := make([]*commonpb.UserId, 5)
	for i := 0; i < 5; i++ {
		users[i] = &commonpb.UserId{Value: []byte(fmt.Sprintf("user%d", i))}

		// Add two tokens for each user
		installId := &commonpb.AppInstallId{Value: fmt.Sprintf("install%d_1", i)}
		err := store.AddToken(ctx, users[i], installId, pushpb.TokenType_FCM_APNS, fmt.Sprintf("token%d_1", i))
		require.NoError(t, err)

		installId = &commonpb.AppInstallId{Value: fmt.Sprintf("install%d_2", i)}
		err = store.AddToken(ctx, users[i], installId, pushpb.TokenType_FCM_APNS, fmt.Sprintf("token%d_2", i))
		require.NoError(t, err)
	}

	// Send visible push to first 3 users
	chatID := model.MustGenerateChatID()
	targetUsers := users[:3]
	data := map[string]string{"my-data": "data is gold"}
	sender := "Test Sender"
	err := pusher.SendPushes(ctx, chatID, targetUsers, "Test Title", "Test Body", &sender, data)
	require.NoError(t, err)

	// Verify the message was sent with all 6 tokens (2 tokens * 3 users)
	require.NotNil(t, fcmClient.sentMessage)
	assert.Len(t, fcmClient.sentMessage.Tokens, 6)

	// Verify title and body are in the data payload
	expectedData := map[string]string{
		"chat_id": base64.StdEncoding.EncodeToString(chatID.Value),
		"my-data": "data is gold",
		"title":   "Test Title",
		"body":    "Test Sender: Test Body",
		"sender":  "Test Sender",
	}
	assert.Equal(t, expectedData, fcmClient.sentMessage.Data)

	// Verify APS content for a visible push
	assert.False(t, fcmClient.sentMessage.APNS.Payload.Aps.ContentAvailable)
	assert.Equal(t, expectedData["title"], fcmClient.sentMessage.APNS.Payload.Aps.Alert.Title)
	assert.Equal(t, expectedData["body"], fcmClient.sentMessage.APNS.Payload.Aps.Alert.Body)

	// Verify the correct tokens were included
	expectedTokens := []string{
		"token0_1", "token0_2",
		"token1_1", "token1_2",
		"token2_1", "token2_2",
	}
	assert.ElementsMatch(t, expectedTokens, fcmClient.sentMessage.Tokens)

	// todo: re-enable when silent push strategy finalized
	if false {
		// Send silent push to the same users
		err = pusher.SendSilentPushes(ctx, chatID, targetUsers, data)
		require.NoError(t, err)

		// Verify silent push message
		require.NotNil(t, fcmClient.sentMessage)
		assert.Len(t, fcmClient.sentMessage.Tokens, 6)

		// Verify data payload remains unchanged
		assert.Equal(t, data, fcmClient.sentMessage.Data)

		// Verify APS content for silent push
		assert.True(t, fcmClient.sentMessage.APNS.Payload.Aps.ContentAvailable)
		assert.Nil(t, fcmClient.sentMessage.APNS.Payload.Aps.Alert)
		assert.Equal(t, base64.StdEncoding.EncodeToString(chatID.Value), fcmClient.sentMessage.APNS.Payload.Aps.ThreadID)
	}
}
