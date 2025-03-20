package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	activitypb "github.com/code-payments/flipchat-protobuf-api/generated/go/activity/v1"

	codekin "github.com/code-payments/code-server/pkg/kin"

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

	t.Run("Get Latest Notifications", func(t *testing.T) {
		var expected []*activitypb.Notification
		for _, tc := range []struct {
			builder       activity.NotificationBuilder
			localizedText string
		}{
			{
				builder:       activity.NewWelcomeBonusNotificationBuilder(context.Background(), userID, codekin.ToQuarks(123), time.Unix(1, 0)),
				localizedText: "You received ⬢\u00A0123\u00A0Kin welcome bonus",
			},
			{
				builder:       activity.NewWeeklyBonusNotificationBuilder(context.Background(), userID, codekin.ToQuarks(456), time.Unix(2, 0)),
				localizedText: "You received ⬢\u00A0456\u00A0Kin weekly bonus",
			},
		} {
			notification, err := activity.SendNotification(context.Background(), activityFeeds, activitypb.ActivityFeedType_TRANSACTION_HISTORY, userID, tc.builder)
			require.NoError(t, err)

			notification.LocalizedText = tc.localizedText
			expected = append([]*activitypb.Notification{notification}, expected...)
		}

		req := &activitypb.GetLatestNotificationsRequest{
			Type: activitypb.ActivityFeedType_TRANSACTION_HISTORY,
		}
		require.NoError(t, keyPair.Auth(req, &req.Auth))

		resp, err := client.GetLatestNotifications(context.Background(), req)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&activitypb.GetLatestNotificationsResponse{Notifications: expected}, resp))

		req = &activitypb.GetLatestNotificationsRequest{
			Type:     activitypb.ActivityFeedType_TRANSACTION_HISTORY,
			MaxItems: int32(len(expected) / 2),
		}
		require.NoError(t, keyPair.Auth(req, &req.Auth))

		resp, err = client.GetLatestNotifications(context.Background(), req)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&activitypb.GetLatestNotificationsResponse{Notifications: expected[:len(expected)/2]}, resp))
	})
}
