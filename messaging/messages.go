package messaging

import (
	"context"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	codekin "github.com/code-payments/code-server/pkg/kin"

	"github.com/code-payments/flipchat-server/profile"
)

var (
	statusMessagePrinter = message.NewPrinter(language.English)
)

type StatusMessageContentBuilder func() (*messagingpb.LocalizedStatusContent, error)

func NewRoomIsLiveStatusContentBuilder(roomNumber uint64) StatusMessageContentBuilder {
	return func() (*messagingpb.LocalizedStatusContent, error) {
		return &messagingpb.LocalizedStatusContent{
			KeyOrText: statusMessagePrinter.Sprintf("This room is live! Tell people to download Flipchat and join room #%d to join this chat", roomNumber),
		}, nil
	}
}

func NewUserJoinedChatStatusContentBuilder(ctx context.Context, profiles profile.Store, userID *commonpb.UserId) StatusMessageContentBuilder {
	return func() (*messagingpb.LocalizedStatusContent, error) {
		profile, err := profiles.GetProfile(ctx, userID)
		if err != nil {
			return nil, err
		}

		return &messagingpb.LocalizedStatusContent{
			KeyOrText: statusMessagePrinter.Sprintf("%s joined", profile.DisplayName),
		}, nil
	}
}

func NewCoverChangedStatusContentBuilder(quarks uint64) StatusMessageContentBuilder {
	return func() (*messagingpb.LocalizedStatusContent, error) {
		return &messagingpb.LocalizedStatusContent{
			KeyOrText: statusMessagePrinter.Sprintf("Cover changed to ⬢ %d Kin", codekin.FromQuarks(quarks)),
		}, nil
	}
}

func NewUserRemovedStatusContentBuilder(ctx context.Context, profiles profile.Store, userID *commonpb.UserId) StatusMessageContentBuilder {
	return func() (*messagingpb.LocalizedStatusContent, error) {
		profile, err := profiles.GetProfile(ctx, userID)
		if err != nil {
			return nil, err
		}

		return &messagingpb.LocalizedStatusContent{
			KeyOrText: statusMessagePrinter.Sprintf("%s was removed", profile.DisplayName),
		}, nil
	}
}
