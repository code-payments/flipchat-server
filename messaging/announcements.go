package messaging

import (
	"context"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	codekin "github.com/code-payments/code-server/pkg/kin"

	"github.com/code-payments/flipchat-server/profile"
)

var (
	announcementPrinter = message.NewPrinter(language.English)
)

type AnnouncementContentBuilder func() (*messagingpb.LocalizedAnnouncementContent, error)

func SendAnnouncement(ctx context.Context, messenger Messenger, chatID *commonpb.ChatId, contentBuilder AnnouncementContentBuilder) error {
	content, err := contentBuilder()
	if err != nil {
		return err
	}
	msg := &messagingpb.Message{
		Content: []*messagingpb.Content{
			{Type: &messagingpb.Content_LocalizedAnnouncement{LocalizedAnnouncement: content}},
		},
		Ts: timestamppb.Now(),
	}
	return messenger.Send(ctx, chatID, msg)
}

func NewRoomIsLiveAnnouncementContentBuilder(roomNumber uint64) AnnouncementContentBuilder {
	return func() (*messagingpb.LocalizedAnnouncementContent, error) {
		return &messagingpb.LocalizedAnnouncementContent{
			KeyOrText: announcementPrinter.Sprintf("This room is live! Tell people to download Flipchat and join room #%d to join this chat", roomNumber),
		}, nil
	}
}

func NewUserJoinedChatAnnouncementContentBuilder(ctx context.Context, profiles profile.Store, userID *commonpb.UserId) AnnouncementContentBuilder {
	return func() (*messagingpb.LocalizedAnnouncementContent, error) {
		profile, err := profiles.GetProfile(ctx, userID)
		if err != nil {
			return nil, err
		}

		return &messagingpb.LocalizedAnnouncementContent{
			KeyOrText: announcementPrinter.Sprintf("%s joined", profile.DisplayName),
		}, nil
	}
}

func NewRoomDisplayNameChangedAnnouncementContentBuilder(displayName string) AnnouncementContentBuilder {
	return func() (*messagingpb.LocalizedAnnouncementContent, error) {
		return &messagingpb.LocalizedAnnouncementContent{
			KeyOrText: announcementPrinter.Sprintf("Room name changed to %s", displayName),
		}, nil
	}
}

func NewCoverChangedAnnouncementContentBuilder(quarks uint64) AnnouncementContentBuilder {
	return func() (*messagingpb.LocalizedAnnouncementContent, error) {
		return &messagingpb.LocalizedAnnouncementContent{
			KeyOrText: announcementPrinter.Sprintf("Cover changed to ⬢ %d Kin", codekin.FromQuarks(quarks)),
		}, nil
	}
}

func NewUserRemovedAnnouncementContentBuilder(ctx context.Context, profiles profile.Store, userID *commonpb.UserId) AnnouncementContentBuilder {
	return func() (*messagingpb.LocalizedAnnouncementContent, error) {
		profile, err := profiles.GetProfile(ctx, userID)
		if err != nil {
			return nil, err
		}

		return &messagingpb.LocalizedAnnouncementContent{
			KeyOrText: announcementPrinter.Sprintf("%s was removed", profile.DisplayName),
		}, nil
	}
}

func NewUserMutedAnnouncementContentBuilder(ctx context.Context, profiles profile.Store, muter, mutee *commonpb.UserId) AnnouncementContentBuilder {
	return func() (*messagingpb.LocalizedAnnouncementContent, error) {
		muterProfile, err := profiles.GetProfile(ctx, muter)
		if err != nil {
			return nil, err
		}

		muteeProfile, err := profiles.GetProfile(ctx, mutee)
		if err != nil {
			return nil, err
		}

		return &messagingpb.LocalizedAnnouncementContent{
			KeyOrText: announcementPrinter.Sprintf("%s muted %s", muterProfile.DisplayName, muteeProfile.DisplayName),
		}, nil
	}
}
