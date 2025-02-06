package messaging

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	codekin "github.com/code-payments/code-server/pkg/kin"

	"github.com/code-payments/flipchat-server/profile"
)

var (
	kinAmountPrinter = message.NewPrinter(language.English)
)

type AnnouncementContentBuilder func() (*messagingpb.Content, error)

func SendAnnouncement(ctx context.Context, messenger Messenger, chatID *commonpb.ChatId, contentBuilder AnnouncementContentBuilder) error {
	content, err := contentBuilder()
	if err != nil {
		return err
	}
	if err := content.Validate(); err != nil {
		return err
	}
	switch content.Type.(type) {
	case *messagingpb.Content_LocalizedAnnouncement, *messagingpb.Content_ActionableAnnouncement:
	default:
		return errors.New("unexpected announcement content type")
	}
	msg := &messagingpb.Message{
		Content: []*messagingpb.Content{
			content,
		},
		Ts: timestamppb.Now(),
	}
	_, err = messenger.Send(ctx, chatID, msg)
	return err
}

func NewRoomIsLiveAnnouncementContentBuilder(roomNumber uint64) AnnouncementContentBuilder {
	return func() (*messagingpb.Content, error) {
		return &messagingpb.Content{
			Type: &messagingpb.Content_LocalizedAnnouncement{
				LocalizedAnnouncement: &messagingpb.LocalizedAnnouncementContent{
					KeyOrText: fmt.Sprintf("This room is live! Tell people to download Flipchat and join room #%d to join this chat", roomNumber),
				},
			},
		}, nil
	}
}

func NewFlipchatIsLiveAnnouncementContentBuilder(chatNumber uint64) AnnouncementContentBuilder {
	return func() (*messagingpb.Content, error) {
		return &messagingpb.Content{
			Type: &messagingpb.Content_ActionableAnnouncement{
				ActionableAnnouncement: &messagingpb.ActionableAnnouncementContent{
					KeyOrText: fmt.Sprintf("This Flipchat is live! Tell people to join Flipchat #%d or share a link on social", chatNumber),
					Action: &messagingpb.ActionableAnnouncementContent_Action{
						Type: &messagingpb.ActionableAnnouncementContent_Action_ShareRoomLink_{
							ShareRoomLink: &messagingpb.ActionableAnnouncementContent_Action_ShareRoomLink{},
						},
					},
				},
			},
		}, nil
	}
}

func NewRoomDisplayNameChangedAnnouncementContentBuilder(roomNumber uint64, displayName string) AnnouncementContentBuilder {
	return func() (*messagingpb.Content, error) {
		return &messagingpb.Content{
			Type: &messagingpb.Content_LocalizedAnnouncement{
				LocalizedAnnouncement: &messagingpb.LocalizedAnnouncementContent{
					KeyOrText: fmt.Sprintf("Room name changed to \"#%d: %s\"", roomNumber, displayName),
				},
			},
		}, nil
	}
}

func NewRoomDisplayNameRemovedAnnouncementContentBuilder() AnnouncementContentBuilder {
	return func() (*messagingpb.Content, error) {
		return &messagingpb.Content{
			Type: &messagingpb.Content_LocalizedAnnouncement{
				LocalizedAnnouncement: &messagingpb.LocalizedAnnouncementContent{
					KeyOrText: "Room name removed",
				},
			},
		}, nil
	}
}

func NewCoverChangedAnnouncementContentBuilder(quarks uint64) AnnouncementContentBuilder {
	return func() (*messagingpb.Content, error) {
		return &messagingpb.Content{
			Type: &messagingpb.Content_LocalizedAnnouncement{
				LocalizedAnnouncement: &messagingpb.LocalizedAnnouncementContent{
					KeyOrText: kinAmountPrinter.Sprintf("Cover changed to ⬢ %d Kin", codekin.FromQuarks(quarks)),
				},
			},
		}, nil
	}
}

func NewMessagingFeeChangedAnnouncementContentBuilder(quarks uint64) AnnouncementContentBuilder {
	return func() (*messagingpb.Content, error) {
		return &messagingpb.Content{
			Type: &messagingpb.Content_LocalizedAnnouncement{
				LocalizedAnnouncement: &messagingpb.LocalizedAnnouncementContent{
					KeyOrText: kinAmountPrinter.Sprintf("Listener Messaging Fee changed to ⬢ %d Kin", codekin.FromQuarks(quarks)),
				},
			},
		}, nil
	}
}

func NewUserPromotedToSpeakerAnnouncementContentBuilder(ctx context.Context, profiles profile.Store, promoter, promotee *commonpb.UserId) AnnouncementContentBuilder {
	return func() (*messagingpb.Content, error) {
		promoterProfile, err := profiles.GetProfile(ctx, promoter)
		if err != nil {
			return nil, err
		}

		promoteeProfile, err := profiles.GetProfile(ctx, promotee)
		if err != nil {
			return nil, err
		}

		return &messagingpb.Content{
			Type: &messagingpb.Content_LocalizedAnnouncement{
				LocalizedAnnouncement: &messagingpb.LocalizedAnnouncementContent{
					KeyOrText: fmt.Sprintf("%s made %s a Speaker", promoterProfile.DisplayName, promoteeProfile.DisplayName),
				},
			},
		}, nil
	}
}

func NewUserRemovedAnnouncementContentBuilder(ctx context.Context, profiles profile.Store, userID *commonpb.UserId) AnnouncementContentBuilder {
	return func() (*messagingpb.Content, error) {
		profile, err := profiles.GetProfile(ctx, userID)
		if err != nil {
			return nil, err
		}

		return &messagingpb.Content{
			Type: &messagingpb.Content_LocalizedAnnouncement{
				LocalizedAnnouncement: &messagingpb.LocalizedAnnouncementContent{
					KeyOrText: fmt.Sprintf("%s was removed", profile.DisplayName),
				},
			},
		}, nil
	}
}

func NewUserMutedAnnouncementContentBuilder(ctx context.Context, profiles profile.Store, muter, mutee *commonpb.UserId) AnnouncementContentBuilder {
	return func() (*messagingpb.Content, error) {
		muterProfile, err := profiles.GetProfile(ctx, muter)
		if err != nil {
			return nil, err
		}

		muteeProfile, err := profiles.GetProfile(ctx, mutee)
		if err != nil {
			return nil, err
		}

		return &messagingpb.Content{
			Type: &messagingpb.Content_LocalizedAnnouncement{
				LocalizedAnnouncement: &messagingpb.LocalizedAnnouncementContent{
					KeyOrText: fmt.Sprintf("%s muted %s", muterProfile.DisplayName, muteeProfile.DisplayName),
				},
			},
		}, nil
	}
}
