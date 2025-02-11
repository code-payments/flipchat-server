package push

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"go.uber.org/zap"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/event"
	"github.com/code-payments/flipchat-server/profile"
)

type EventHandler struct {
	log      *zap.Logger
	chats    chat.Store
	profiles profile.Store
	pusher   Pusher
}

func NewPushEventHandler(
	log *zap.Logger,
	chats chat.Store,
	profiles profile.Store,
	pusher Pusher,
) *EventHandler {
	return &EventHandler{
		log:      log,
		chats:    chats,
		profiles: profiles,
		pusher:   pusher,
	}
}

func (h *EventHandler) OnEvent(chatID *commonpb.ChatId, e *event.ChatEvent) {
	ctx := context.Background()

	if e.MessageUpdate != nil {
		h.log.Debug("Handling push for message", zap.String("chat_id", base64.StdEncoding.EncodeToString(chatID.Value)))
		if err := h.handleMessage(ctx, chatID, e.MessageUpdate); err != nil {
			h.log.Warn("Failed to handle message", zap.String("chat_id", base64.StdEncoding.EncodeToString(chatID.Value)), zap.Error(err))
		}

		h.log.Debug("Processed message update")
	}

	// TODO: Handle member updates (when we know about join/leave).
	// TODO: This probably means that the event contains the delta,
	//       and it's up to the streamer to factor it out?
}

func (h *EventHandler) handleMessage(ctx context.Context, chatID *commonpb.ChatId, msg *messagingpb.Message) error {
	if msg.SenderId == nil {
		h.log.Debug("Dropping push, no sender")
		return nil
	}
	if len(msg.Content) == 0 {
		h.log.Debug("Dropping push, no content")
		return nil
	}
	// todo: filter by speakers?
	if msg.WasSenderOffStage {
		h.log.Debug("Dropping push, sender was off stage")
		return nil
	}

	sender, err := h.profiles.GetProfile(ctx, msg.SenderId)
	if errors.Is(err, profile.ErrNotFound) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get sender profile: %w", err)
	}

	md, err := h.chats.GetChatMetadata(ctx, chatID)
	if err != nil {
		return fmt.Errorf("failed to get chat: %w", err)
	}

	members, err := h.chats.GetMembers(ctx, chatID)
	if err != nil {
		return fmt.Errorf("failed to get chat members: %w", err)
	}

	pushMembers := make([]*commonpb.UserId, 0, len(members))
	for _, member := range members {
		if !member.IsPushEnabled {
			continue
		}
		if bytes.Equal(member.UserID.Value, msg.SenderId.Value) {
			continue
		}
		pushMembers = append(pushMembers, member.UserID)
	}

	if len(pushMembers) == 0 {
		h.log.Debug("Dropping push, no pushable members")
		return nil
	}

	var pushPreview string
	switch typed := msg.Content[0].Type.(type) {
	case *messagingpb.Content_Text:
		pushPreview = typed.Text.Text
	case *messagingpb.Content_Reply:
		// todo: this needs tests
		pushPreview = typed.Reply.ReplyText
	default:
		return nil
	}

	var title string
	switch md.Type {
	case chatpb.Metadata_GROUP:
		title = fmt.Sprintf("#%d", md.RoomNumber)
		if len(md.DisplayName) > 0 {
			title = fmt.Sprintf("#%d: %s", md.RoomNumber, md.DisplayName)
		}
	case chatpb.Metadata_TWO_WAY:
		title = sender.DisplayName
	}

	data := make(map[string]string)
	if err := h.pusher.SendPushes(ctx, chatID, pushMembers, title, pushPreview, &sender.DisplayName, data); err != nil {
		h.log.Warn("Failed to send pushes", zap.String("chat_id", base64.StdEncoding.EncodeToString(chatID.Value)), zap.Error(err))
	}

	return nil
}
