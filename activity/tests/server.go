package tests

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	activitypb "github.com/code-payments/flipchat-protobuf-api/generated/go/activity/v1"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/activity"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/profile"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/testutil"
)

func RunServerTests(t *testing.T, accounts account.Store, activityFeeds activity.Store, chats chat.Store, profiles profile.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, accounts account.Store, activityFeeds activity.Store, chats chat.Store, profiles profile.Store){
		testActivityServer_HappyPath,
	} {
		tf(t, accounts, activityFeeds, chats, profiles)
		teardown()
	}
}

func testActivityServer_HappyPath(t *testing.T, accounts account.Store, activityFeeds activity.Store, chats chat.Store, profiles profile.Store) {
	log := zap.Must(zap.NewDevelopment())

	userID := model.MustGenerateUserID()
	keyPair := model.MustGenerateKeyPair()
	_, _ = accounts.Bind(context.Background(), userID, keyPair.Proto())
	_ = accounts.SetRegistrationFlag(context.Background(), userID, true)

	serv := activity.NewServer(
		log,
		account.NewAuthorizer(log, accounts, auth.NewKeyPairAuthenticator()),
		activityFeeds,
		chats,
		profiles,
	)

	cc := testutil.RunGRPCServer(t, testutil.WithService(func(s *grpc.Server) {
		activitypb.RegisterActivityFeedServer(s, serv)
	}))

	client := activitypb.NewActivityFeedClient(cc)

	t.Run("Empty", func(t *testing.T) {
		req := &activitypb.GetLatestNotificationsRequest{
			Type:     activitypb.ActivityFeedType_TRANSACTION_HISTORY,
			MaxItems: 100,
		}
		require.NoError(t, keyPair.Auth(req, &req.Auth))

		resp, err := client.GetLatestNotifications(context.Background(), req)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&activitypb.GetLatestNotificationsResponse{}, resp))
	})
}
