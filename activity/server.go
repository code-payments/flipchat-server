package activity

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	activitypb "github.com/code-payments/flipchat-protobuf-api/generated/go/activity/v1"

	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/profile"
)

const (
	defaultMaxNotifications = 256
)

type Server struct {
	log           *zap.Logger
	authz         auth.Authorizer
	activityFeeds Store
	chats         chat.Store
	profiles      profile.Store

	activitypb.UnimplementedActivityFeedServer
}

func NewServer(
	log *zap.Logger,
	authz auth.Authorizer,
	activityFeeds Store,
	chats chat.Store,
	profiles profile.Store,
) *Server {
	return &Server{
		log:           log,
		authz:         authz,
		activityFeeds: activityFeeds,
		chats:         chats,
		profiles:      profiles,
	}
}

func (s *Server) GetLatestNotifications(ctx context.Context, req *activitypb.GetLatestNotificationsRequest) (*activitypb.GetLatestNotificationsResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("activity_feed_type", req.Type.String()),
	)

	limit := defaultMaxNotifications
	if req.MaxItems > 0 {
		limit = int(req.MaxItems)
	}

	notifications, err := s.activityFeeds.GetLatestNotifications(ctx, req.Type, userID, limit)
	if err != nil {
		log.Warn("Failed to get notifications", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get notifications")
	}

	notificationsWithLocalizedText := make([]*activitypb.Notification, 0)
	for _, notification := range notifications {
		log = log.With(zap.String("notification_id", NotificationIDString(notification.Id)))

		err = InjectLocalizedText(ctx, s.chats, s.profiles, notification)
		if err != nil {
			log.Warn("Failed to inject localized notification text", zap.Error(err))
			continue
		}
		notificationsWithLocalizedText = append(notificationsWithLocalizedText, notification)
	}

	return &activitypb.GetLatestNotificationsResponse{Notifications: notificationsWithLocalizedText}, nil
}
