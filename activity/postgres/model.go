package postgres

import (
	"context"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	activitypb "github.com/code-payments/flipchat-protobuf-api/generated/go/activity/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	"github.com/code-payments/flipchat-server/activity"
	pg "github.com/code-payments/flipchat-server/database/postgres"
)

const (
	activityFeedsTableName = "flipchat_activity_feeds"
	allActivityFeedFields  = `"id", "userId", "activityFeedType", "notificationType", "count", "chatId", "messageId", "ts", "createdAt", "updatedAt"`
)

type model struct {
	ID               string    `db:"id"`
	UserID           string    `db:"userId"`
	ActivityFeedType int       `db:"activityFeedType"`
	NotificationType int       `db:"notificationType"`
	Count            int64     `db:"count"`
	ChatID           *string   `db:"chatId"`
	MessageID        *string   `db:"messageId"`
	Ts               time.Time `db:"ts"`
	CreatedAt        time.Time `db:"createdAt"`
	UpdatedAt        time.Time `db:"updatedAt"`
}

func toModel(activityFeedType activitypb.ActivityFeedType, userID *commonpb.UserId, notification *activitypb.Notification) (*model, error) {
	if activityFeedType != activitypb.ActivityFeedType_TRANSACTION_HISTORY {
		return nil, activity.ErrInvalidActivityFeedType
	}

	var notificationType activity.NotificationType
	var count int64
	var chatID *string
	var messageID *string

	switch typed := notification.AdditionalMetadata.(type) {
	case *activitypb.Notification_WelcomeBonus:
		notificationType = activity.NotificationTypeWelcomeBonus
		count = int64(typed.WelcomeBonus.QuarksReceived)
	case *activitypb.Notification_WeeklyBonus:
		notificationType = activity.NotificationTypeWeeklyBonus
		count = int64(typed.WeeklyBonus.QuarksReceived)
	default:
		return nil, activity.ErrInvalidNotificationType
	}

	return &model{
		ID:               pg.Encode(notification.Id.Value, pg.Hex),
		UserID:           pg.Encode(userID.Value),
		ActivityFeedType: int(activityFeedType),
		NotificationType: int(notificationType),
		Count:            count,
		ChatID:           chatID,
		MessageID:        messageID,
		Ts:               notification.Ts.AsTime(),
	}, nil
}

func fromModel(m *model) (*activitypb.Notification, error) {
	decodedId, err := pg.Decode(m.ID)
	if err != nil {
		return nil, err
	}

	baseNotification := &activitypb.Notification{
		Id: &activitypb.NotificationId{
			Value: decodedId,
		},
		Ts: timestamppb.New(m.Ts),
	}

	switch m.NotificationType {
	case activity.NotificationTypeWelcomeBonus:
		baseNotification.AdditionalMetadata = &activitypb.Notification_WelcomeBonus{
			WelcomeBonus: &activitypb.WelcomeBonusNotificationMetadata{
				QuarksReceived: uint64(m.Count),
			},
		}
	case activity.NotificationTypeWeeklyBonus:
		baseNotification.AdditionalMetadata = &activitypb.Notification_WeeklyBonus{
			WeeklyBonus: &activitypb.WeeklyBonusNotificationMetadata{
				QuarksReceived: uint64(m.Count),
			},
		}
	default:
		return nil, activity.ErrInvalidNotificationType
	}

	return baseNotification, nil
}

func (m *model) dbSave(ctx context.Context, pool *pgxpool.Pool) error {
	query := `INSERT INTO ` + activityFeedsTableName + `(` + allActivityFeedFields + `) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW()) ON CONFLICT ("id") DO NOTHING RETURNING ` + allActivityFeedFields
	return pgxscan.Get(
		ctx,
		pool,
		m,
		query,
		m.ID,
		m.UserID,
		m.ActivityFeedType,
		m.NotificationType,
		m.Count,
		m.ChatID,
		m.MessageID,
		m.Ts,
	)
}

func dbGetLatestNotifications(ctx context.Context, pool *pgxpool.Pool, activityFeedType activitypb.ActivityFeedType, userID *commonpb.UserId, limit int) ([]*model, error) {
	var res []*model
	query := `SELECT ` + allActivityFeedFields + ` FROM ` + activityFeedsTableName + ` WHERE "userId" = $1 AND "activityFeedType" = $2 ORDER BY "ts" DESC LIMIT $3`
	err := pgxscan.Select(
		ctx,
		pool,
		&res,
		query,
		pg.Encode(userID.Value),
		activityFeedType,
		limit,
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return res, nil
}
