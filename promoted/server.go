package promoted

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	promotedpb "github.com/code-payments/flipchat-protobuf-api/generated/go/promoted/v1"

	"github.com/code-payments/flipchat-server/auth"
)

// Server implements the Promoted service.
type Server struct {
	log      *zap.Logger
	promoted Store
	authz    auth.Authorizer

	promotedpb.UnimplementedPromotedServer
}

// NewServer creates a new Promoted server.
func NewServer(log *zap.Logger, promoted Store, authz auth.Authorizer) *Server {
	return &Server{
		log:      log,
		promoted: promoted,
		authz:    authz,
	}
}

// GetPromotedChats handles the GetPromotedChats RPC.
func (s *Server) GetPromotedChats(ctx context.Context, req *promotedpb.GetPromotedChatsRequest) (*promotedpb.GetPromotedChatsResponse, error) {
	// Validate request
	if len(req.Topic) < 1 || len(req.Topic) > 100 {
		return &promotedpb.GetPromotedChatsResponse{
			Result: promotedpb.GetPromotedChatsResponse_INVALID_REQUEST,
		}, nil
	}

	// For now, assuming no authorization is required.

	res, err := s.promoted.GetPromotedChats(ctx, req.Topic)
	if err != nil {
		s.log.Error("Failed to get promoted chats", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to retrieve promoted chats")
	}

	var chatIDs []*commonpb.ChatId
	for _, promoted := range res {
		chatIDs = append(chatIDs, promoted.ChatID)
	}

	return &promotedpb.GetPromotedChatsResponse{
		Result: promotedpb.GetPromotedChatsResponse_OK,
		Chats:  chatIDs,
	}, nil
}
