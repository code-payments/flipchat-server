package rpc

import (
	"context"
	"errors"

	"go.uber.org/zap"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/event"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/profile"
)

// todo: this needs tests
type ProfileEventGenerator struct {
	log      *zap.Logger
	chats    chat.Store
	profiles profile.Store
	eventBus *event.Bus[*commonpb.ChatId, *event.ChatEvent]
}

func NewProfileEventGenerator(log *zap.Logger, chats chat.Store, profiles profile.Store, eventBus *event.Bus[*commonpb.ChatId, *event.ChatEvent]) event.ProfileGenerator {
	return &ProfileEventGenerator{
		log:      log,
		chats:    chats,
		profiles: profiles,
		eventBus: eventBus,
	}
}

func (e *ProfileEventGenerator) OnProfileUpdated(ctx context.Context, userID *commonpb.UserId) {
	log := e.log.With(
		zap.String("user_id", model.UserIDString(userID)),
	)

	profile, err := e.profiles.GetProfile(ctx, userID)
	if err != nil {
		log.Warn("Failed to get user profile")
		return
	}

	chatIDs, err := e.chats.GetChatsForUser(ctx, userID)
	if errors.Is(err, chat.ErrChatNotFound) {
		return
	} else if err != nil {
		log.Warn("Failed to get chats for user")
	}

	for _, chatID := range chatIDs {
		err = e.eventBus.OnEvent(chatID, &event.ChatEvent{ChatID: chatID, MemberUpdates: []*chatpb.MemberUpdate{
			{
				Kind: &chatpb.MemberUpdate_IdentityChanged_{
					IdentityChanged: &chatpb.MemberUpdate_IdentityChanged{
						Member: userID,
						NewIdentity: &chatpb.MemberIdentity{
							DisplayName:    profile.GetDisplayName(),
							SocialProfiles: profile.GetSocialProfiles(),
						},
					},
				},
			},
		}})
		if err != nil {
			log.Warn("Failed to notify member identity changed", zap.Error(err))
		}
	}
}
