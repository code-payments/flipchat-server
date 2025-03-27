package activity

import (
	"context"
	"errors"

	activitypb "github.com/code-payments/flipchat-protobuf-api/generated/go/activity/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

var (
	ErrInvalidActivityFeedType = errors.New("activity feed type not supported")
	ErrInvalidNotificationType = errors.New("notification type not supported")
)

type Store interface {
	// SaveNotification inserts or updates a notification depending on whether
	// the notification supports grouping
	SaveNotification(ctx context.Context, activityFeedType activitypb.ActivityFeedType, userID *commonpb.UserId, notification *activitypb.Notification) (*activitypb.Notification, error)

	// GetLatestNotifications gets the latest N notifications for a user's
	// activity feed type
	GetLatestNotifications(ctx context.Context, activityFeedType activitypb.ActivityFeedType, userID *commonpb.UserId, limit int) ([]*activitypb.Notification, error)
}
