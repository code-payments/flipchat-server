package push

import (
	"context"
	"fmt"
	"testing"

	"firebase.google.com/go/v4/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	pushpb "github.com/code-payments/flipchat-protobuf-api/generated/go/push/v1"
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

func TestFCMPusher_SendPush(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	fcmClient := &testFCMClient{}
	pusher := NewFCMPusher(zap.NewNop(), store, fcmClient)

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

	// Send push to first 3 users
	targetUsers := users[:3]
	data := map[string]string{"my-data": "data is gold"}
	err := pusher.SendPushes(ctx, targetUsers, "Test Title", "Test Body", data)
	require.NoError(t, err)

	// Verify the message was sent with all 6 tokens (2 tokens * 3 users)
	require.NotNil(t, fcmClient.sentMessage)
	assert.Len(t, fcmClient.sentMessage.Tokens, 6)

	// Verify message content
	assert.Equal(t, "Test Title", fcmClient.sentMessage.Notification.Title)
	assert.Equal(t, "Test Body", fcmClient.sentMessage.Notification.Body)
	assert.Equal(t, data, fcmClient.sentMessage.Data)

	// Verify the correct tokens were included
	expectedTokens := []string{
		"token0_1", "token0_2",
		"token1_1", "token1_2",
		"token2_1", "token2_2",
	}
	assert.ElementsMatch(t, expectedTokens, fcmClient.sentMessage.Tokens)
}
