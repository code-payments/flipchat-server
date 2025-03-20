package activity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"

	activitypb "github.com/code-payments/flipchat-protobuf-api/generated/go/activity/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

type NotificationType uint32

const (
	NotificationTypeUnknown = iota
	NotificationTypeWelcomeBonus
	NotificationTypeWeeklyBonus
	NotificationTypeCreateGroup
	NotificationTypeSendListenerMessage
	NotificationTypeSendTip
	NotificationTypeReceivedTip
	NotificationTypePromotedToSpeaker
)

func NotificationIDString(id *activitypb.NotificationId) string {
	if id == nil {
		return "<invalid>"
	}
	return hex.EncodeToString(id.Value)
}

func GetNotificationID(notificationType NotificationType, userID *commonpb.UserId, additionalSeeds ...[]byte) (*activitypb.NotificationId, error) {
	if notificationType == NotificationTypeUnknown {
		return nil, errors.New("notification type cannot be unknown")
	}

	var notificationTypeBytes [4]byte
	binary.LittleEndian.PutUint32(notificationTypeBytes[:], uint32(notificationType))

	hasher := sha256.New()
	_, err := hasher.Write(notificationTypeBytes[:])
	if err != nil {
		return nil, err
	}
	_, err = hasher.Write(userID.Value)
	if err != nil {
		return nil, err
	}
	for _, seed := range additionalSeeds {
		_, err = hasher.Write(seed)
		if err != nil {
			return nil, err
		}
	}
	hashed := hasher.Sum(nil)

	return &activitypb.NotificationId{Value: hashed}, nil
}
