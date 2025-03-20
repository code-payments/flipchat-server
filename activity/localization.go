package activity

import (
	"context"
	"errors"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	activitypb "github.com/code-payments/flipchat-protobuf-api/generated/go/activity/v1"

	codekin "github.com/code-payments/code-server/pkg/kin"

	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/profile"
)

var (
	kinAmountPrinter = message.NewPrinter(language.English)
)

func InjectLocalizedText(ctx context.Context, chats chat.Store, profiles profile.Store, notification *activitypb.Notification) error {
	var localizedText string
	switch typed := notification.AdditionalMetadata.(type) {
	case *activitypb.Notification_WelcomeBonus:
		localizedText = kinAmountPrinter.Sprintf("You received ⬢\u00A0%d\u00A0Kin welcome bonus", codekin.FromQuarks(typed.WelcomeBonus.QuarksReceived))
	case *activitypb.Notification_WeeklyBonus:
		localizedText = kinAmountPrinter.Sprintf("You received ⬢\u00A0%d\u00A0Kin weekly bonus", codekin.FromQuarks(typed.WeeklyBonus.QuarksReceived))
	case *activitypb.Notification_CreateGroup:
		localizedText = kinAmountPrinter.Sprintf("You paid ⬢\u00A0%d\u00A0Kin to create a new Flipchat", codekin.FromQuarks(typed.CreateGroup.QuarksSpent))
	case *activitypb.Notification_SendListenerMessage:
		localizedText = kinAmountPrinter.Sprintf("You paid ⬢\u00A0%d\u00A0Kin", codekin.FromQuarks(typed.SendListenerMessage.QuarksSpent))
	case *activitypb.Notification_SendTip:
		localizedText = kinAmountPrinter.Sprintf("You tipped ⬢\u00A0%d\u00A0Kin", codekin.FromQuarks(typed.SendTip.TotalQuarksSent))
	case *activitypb.Notification_ReceivedTip:
		localizedText = kinAmountPrinter.Sprintf("You received ⬢\u00A0%d\u00A0Kin", codekin.FromQuarks(typed.ReceivedTip.TotalQuarksReceived))
	case *activitypb.Notification_PromotedToSpeaker:
		profile, err := profiles.GetProfile(ctx, typed.PromotedToSpeaker.PromtedBy)
		if err != nil {
			return err
		}

		chatMd, err := chats.GetChatMetadata(ctx, typed.PromotedToSpeaker.ChatId)
		if err != nil {
			return err
		}

		if len(profile.DisplayName) == 0 {
			return errors.New("user doesn't have a display name")
		}
		if len(chatMd.DisplayName) == 0 {
			return errors.New("chat doesn't have a display name")
		}

		localizedText = kinAmountPrinter.Sprintf("%s made you a speaker in %s", profile.DisplayName, chatMd.DisplayName)
	default:
		return errors.New("unsupported notification type")
	}
	notification.LocalizedText = localizedText
	return nil
}
