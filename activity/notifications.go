package activity

import (
	"context"
	"encoding/binary"
	"errors"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	activitypb "github.com/code-payments/flipchat-protobuf-api/generated/go/activity/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
)

type NotificationBuilder func() (*activitypb.Notification, error)

// SendNotification sends a notification to a user's activity feed
func SendNotification(ctx context.Context, activityFeeds Store, activityFeedType activitypb.ActivityFeedType, userID *commonpb.UserId, builder NotificationBuilder) (*activitypb.Notification, error) {
	if activityFeedType != activitypb.ActivityFeedType_TRANSACTION_HISTORY {
		return nil, errors.New("unsupported activity feed type")
	}

	notification, err := builder()
	if err != nil {
		return nil, err
	}

	if len(notification.LocalizedText) > 0 {
		return nil, errors.New("cannot set localized text")
	}

	notification.LocalizedText = "placeholder" // To pass proto validation
	if err := notification.Validate(); err != nil {
		return nil, err
	}
	notification.LocalizedText = ""

	return activityFeeds.SaveNotification(ctx, activityFeedType, userID, notification)
}

func NewWelcomeBonusNotificationBuilder(ctx context.Context, userID *commonpb.UserId, quarks uint64, ts time.Time) NotificationBuilder {
	return func() (*activitypb.Notification, error) {
		id, err := GetNotificationID(NotificationTypeWelcomeBonus, userID)
		if err != nil {
			return nil, err
		}

		return &activitypb.Notification{
			Id: id,
			Ts: timestamppb.New(ts),
			AdditionalMetadata: &activitypb.Notification_WelcomeBonus{
				WelcomeBonus: &activitypb.WelcomeBonusNotificationMetadata{
					QuarksReceived: quarks,
				},
			},
		}, nil
	}
}

func NewWeeklyBonusNotificationBuilder(ctx context.Context, userID *commonpb.UserId, quarks uint64, ts time.Time) NotificationBuilder {
	return func() (*activitypb.Notification, error) {
		var yearBytes [4]byte
		var weekBytes [4]byte
		year, week := ts.ISOWeek()
		binary.LittleEndian.PutUint32(yearBytes[:], uint32(year))
		binary.LittleEndian.PutUint32(weekBytes[:], uint32(week))

		id, err := GetNotificationID(NotificationTypeWeeklyBonus, userID, yearBytes[:], weekBytes[:])
		if err != nil {
			return nil, err
		}

		return &activitypb.Notification{
			Id: id,
			Ts: timestamppb.New(ts),
			AdditionalMetadata: &activitypb.Notification_WeeklyBonus{
				WeeklyBonus: &activitypb.WeeklyBonusNotificationMetadata{
					QuarksReceived: quarks,
				},
			},
		}, nil
	}
}

func NewCreateGroupsNotificationBuilder(ctx context.Context, userID *commonpb.UserId, chatID *commonpb.ChatId, quarks uint64, ts time.Time) NotificationBuilder {
	return func() (*activitypb.Notification, error) {
		id, err := GetNotificationID(NotificationTypeCreateGroup, userID, chatID.Value)
		if err != nil {
			return nil, err
		}

		return &activitypb.Notification{
			Id: id,
			Ts: timestamppb.New(ts),
			AdditionalMetadata: &activitypb.Notification_CreateGroup{
				CreateGroup: &activitypb.CreateGroupNotificationMetadata{
					ChatId:      chatID,
					QuarksSpent: quarks,
				},
			},
		}, nil
	}
}

func NewSendListenerMessageNotificationBuilder(ctx context.Context, userID *commonpb.UserId, chatID *commonpb.ChatId, messageID *messagingpb.MessageId, quarks uint64, ts time.Time) NotificationBuilder {
	return func() (*activitypb.Notification, error) {
		id, err := GetNotificationID(NotificationTypeSendListenerMessage, userID, chatID.Value, messageID.Value)
		if err != nil {
			return nil, err
		}

		return &activitypb.Notification{
			Id: id,
			Ts: timestamppb.New(ts),
			AdditionalMetadata: &activitypb.Notification_SendListenerMessage{
				SendListenerMessage: &activitypb.SendListenerMessageNotificationMetadata{
					ChatId:      chatID,
					MessageId:   messageID,
					QuarksSpent: quarks,
				},
			},
		}, nil
	}
}

func NewSendTipNotificationBuilder(ctx context.Context, userID *commonpb.UserId, chatID *commonpb.ChatId, messageID *messagingpb.MessageId, quarks uint64, ts time.Time) NotificationBuilder {
	return func() (*activitypb.Notification, error) {
		id, err := GetNotificationID(NotificationTypeSendTip, userID, chatID.Value, messageID.Value)
		if err != nil {
			return nil, err
		}

		return &activitypb.Notification{
			Id: id,
			Ts: timestamppb.New(ts),
			AdditionalMetadata: &activitypb.Notification_SendTip{
				SendTip: &activitypb.SendTipNotificationMetadata{
					ChatId:          chatID,
					MessageId:       messageID,
					TotalQuarksSent: quarks,
				},
			},
		}, nil
	}
}

func NewReceivedTipNotificationBuilder(ctx context.Context, userID *commonpb.UserId, chatID *commonpb.ChatId, messageID *messagingpb.MessageId, quarks uint64, ts time.Time) NotificationBuilder {
	return func() (*activitypb.Notification, error) {
		id, err := GetNotificationID(NotificationTypeReceivedTip, userID, chatID.Value, messageID.Value)
		if err != nil {
			return nil, err
		}

		return &activitypb.Notification{
			Id: id,
			Ts: timestamppb.New(ts),
			AdditionalMetadata: &activitypb.Notification_ReceivedTip{
				ReceivedTip: &activitypb.ReceivedTipNotificationMetadata{
					ChatId:              chatID,
					MessageId:           messageID,
					TotalQuarksReceived: quarks,
				},
			},
		}, nil
	}
}

func NewPromotedToSpeakerNotificationBuilder(ctx context.Context, promotee, promoter *commonpb.UserId, chatID *commonpb.ChatId, ts time.Time) NotificationBuilder {
	return func() (*activitypb.Notification, error) {
		id, err := GetNotificationID(NotificationTypePromotedToSpeaker, promotee, chatID.Value, promoter.Value)
		if err != nil {
			return nil, err
		}

		return &activitypb.Notification{
			Id: id,
			Ts: timestamppb.New(ts),
			AdditionalMetadata: &activitypb.Notification_PromotedToSpeaker{
				PromotedToSpeaker: &activitypb.PromotedToSpeakerNotificationMetadata{
					ChatId:    chatID,
					PromtedBy: promoter,
				},
			},
		}, nil
	}
}
