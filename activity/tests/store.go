package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	activitypb "github.com/code-payments/flipchat-protobuf-api/generated/go/activity/v1"

	"github.com/code-payments/flipchat-server/activity"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"
)

func RunStoreTests(t *testing.T, s activity.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s activity.Store){
		testActivityStore_HappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testActivityStore_HappyPath(t *testing.T, store activity.Store) {
	userID := model.MustGenerateUserID()
	var allExpected []*activitypb.Notification

	t.Run("Empty", func(t *testing.T) {
		actual, err := store.GetLatestNotifications(context.Background(), activitypb.ActivityFeedType_TRANSACTION_HISTORY, userID, 10)
		require.NoError(t, err)
		require.Empty(t, actual)
	})

	t.Run("Welcome Bonus", func(t *testing.T) {
		expected, err := activity.NewWelcomeBonusNotificationBuilder(context.Background(), userID, 123, time.Unix(1, 0))()
		require.NoError(t, err)
		allExpected = append([]*activitypb.Notification{expected}, allExpected...)

		actual, err := store.SaveNotification(context.Background(), activitypb.ActivityFeedType_TRANSACTION_HISTORY, userID, expected)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(expected, actual))

		allActual, err := store.GetLatestNotifications(context.Background(), activitypb.ActivityFeedType_TRANSACTION_HISTORY, userID, 1)
		require.NoError(t, err)
		require.Len(t, allActual, 1)
		require.NoError(t, protoutil.ProtoEqualError(expected, allActual[0]))

		other, err := activity.NewWelcomeBonusNotificationBuilder(context.Background(), userID, 42, time.Unix(123456789, 0))()
		require.NoError(t, err)

		actual, err = store.SaveNotification(context.Background(), activitypb.ActivityFeedType_TRANSACTION_HISTORY, userID, other)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(expected, actual))

		allActual, err = store.GetLatestNotifications(context.Background(), activitypb.ActivityFeedType_TRANSACTION_HISTORY, userID, 1)
		require.NoError(t, err)
		require.Len(t, allActual, 1)
		require.NoError(t, protoutil.ProtoEqualError(expected, allActual[0]))
	})

	t.Run("Weekly Bonus", func(t *testing.T) {
		expected, err := activity.NewWeeklyBonusNotificationBuilder(context.Background(), userID, 456, time.Unix(2, 0))()
		require.NoError(t, err)
		allExpected = append([]*activitypb.Notification{expected}, allExpected...)

		actual, err := store.SaveNotification(context.Background(), activitypb.ActivityFeedType_TRANSACTION_HISTORY, userID, expected)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(expected, actual))

		allActual, err := store.GetLatestNotifications(context.Background(), activitypb.ActivityFeedType_TRANSACTION_HISTORY, userID, 1)
		require.NoError(t, err)
		require.Len(t, allActual, 1)
		require.NoError(t, protoutil.ProtoEqualError(expected, allActual[0]))

		other, err := activity.NewWeeklyBonusNotificationBuilder(context.Background(), userID, 42, time.Unix(3, 0))()
		require.NoError(t, err)

		actual, err = store.SaveNotification(context.Background(), activitypb.ActivityFeedType_TRANSACTION_HISTORY, userID, other)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(expected, actual))

		allActual, err = store.GetLatestNotifications(context.Background(), activitypb.ActivityFeedType_TRANSACTION_HISTORY, userID, 1)
		require.NoError(t, err)
		require.Len(t, allActual, 1)
		require.NoError(t, protoutil.ProtoEqualError(expected, allActual[0]))
	})

	t.Run("Get Latest Notifications", func(t *testing.T) {
		allActual, err := store.GetLatestNotifications(context.Background(), activitypb.ActivityFeedType_TRANSACTION_HISTORY, userID, 100)
		require.NoError(t, err)
		require.NoError(t, protoutil.SliceEqualError(allExpected, allActual))

		allActual, err = store.GetLatestNotifications(context.Background(), activitypb.ActivityFeedType_TRANSACTION_HISTORY, userID, len(allExpected)/2)
		require.NoError(t, err)
		require.NoError(t, protoutil.SliceEqualError(allExpected[:len(allExpected)/2], allActual))
	})

	t.Run("Invalid Activity Feed", func(t *testing.T) {
		expected, err := activity.NewWelcomeBonusNotificationBuilder(context.Background(), userID, 123, time.Unix(1, 0))()
		require.NoError(t, err)

		_, err = store.SaveNotification(context.Background(), activitypb.ActivityFeedType_UNKNOWN, userID, expected)
		require.Equal(t, activity.ErrInvalidActivityFeedType, err)

		_, err = store.GetLatestNotifications(context.Background(), activitypb.ActivityFeedType_UNKNOWN, userID, 100)
		require.Equal(t, activity.ErrInvalidActivityFeedType, err)
	})

	t.Run("Invalid Notification", func(t *testing.T) {
		_, err := store.SaveNotification(context.Background(), activitypb.ActivityFeedType_TRANSACTION_HISTORY, userID, &activitypb.Notification{
			Id:                 &activitypb.NotificationId{Value: make([]byte, 32)},
			AdditionalMetadata: nil,
			Ts:                 timestamppb.Now(),
		})
		require.Equal(t, activity.ErrInvalidNotificationType, err)
	})
}
