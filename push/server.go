package push

import (
	"context"
	"slices"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pushpb "github.com/code-payments/flipchat-protobuf-api/generated/go/push/v1"

	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/model"
)

type Server struct {
	log    *zap.Logger
	auth   auth.Authorizer
	tokens TokenStore

	pushpb.UnimplementedPushServer
}

func NewServer(log *zap.Logger, auth auth.Authorizer, tokens TokenStore) *Server {
	return &Server{
		log:    log,
		auth:   auth,
		tokens: tokens,
	}
}

func (s *Server) AddToken(ctx context.Context, req *pushpb.AddTokenRequest) (*pushpb.AddTokenResponse, error) {
	userID, err := s.auth.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	if err = s.tokens.AddToken(ctx, userID, req.AppInstall, req.TokenType, req.PushToken); err != nil {
		s.log.Warn("Failed to add push token", zap.String("user_id", model.UserIDString(userID)), zap.Error(err))
		return nil, status.Errorf(codes.Internal, "")
	}

	return &pushpb.AddTokenResponse{}, nil
}

func (s *Server) DeleteToken(ctx context.Context, req *pushpb.DeleteTokenRequest) (*pushpb.DeleteTokenResponse, error) {
	userID, err := s.auth.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(zap.String("user_id", model.UserIDString(userID)), zap.String("push_token", req.PushToken))

	// TODO: Verify that the user owns the token? If not...I mean it just makes sense...
	existing, err := s.tokens.GetTokens(ctx, userID)
	if err != nil {
		log.Warn("Failed to get push tokens", zap.Error(err))
		return nil, status.Error(codes.Internal, "")
	}

	exists := slices.ContainsFunc(existing, func(token Token) bool {
		return token.Type == req.TokenType && token.Token == req.PushToken
	})
	if !exists {
		log.Info("Did not delete push token (not found)")
		return &pushpb.DeleteTokenResponse{}, nil
	}

	if err = s.tokens.DeleteToken(ctx, req.TokenType, req.PushToken); err != nil {
		log.Warn("Failed to get push tokens", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to delete push token")
	}

	return &pushpb.DeleteTokenResponse{}, nil
}
