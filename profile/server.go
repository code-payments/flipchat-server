package profile

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	profilepb "github.com/code-payments/flipchat-protobuf-api/generated/go/profile/v1"

	"github.com/code-payments/flipchat-server/auth"
)

type Server struct {
	log   *zap.Logger
	store Store
	authz auth.Authorizer

	profilepb.UnimplementedProfileServer
}

func NewServer(log *zap.Logger, store Store, authz auth.Authorizer) *Server {
	return &Server{
		log:   log,
		store: store,
		authz: authz,
	}
}

func (s *Server) GetProfile(ctx context.Context, req *profilepb.GetProfileRequest) (*profilepb.GetProfileResponse, error) {
	profile, err := s.store.GetProfile(ctx, req.UserId)
	if errors.Is(err, ErrNotFound) {
		return &profilepb.GetProfileResponse{Result: profilepb.GetProfileResponse_NOT_FOUND}, nil
	} else if err != nil {
		s.log.Warn("Failed to get profile", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get profile")
	}

	return &profilepb.GetProfileResponse{UserProfile: profile}, nil
}

func (s *Server) SetDisplayName(ctx context.Context, req *profilepb.SetDisplayNameRequest) (*profilepb.SetDisplayNameResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	if err := s.store.SetDisplayName(ctx, userID, req.DisplayName); err != nil {
		if errors.Is(err, ErrInvalidDisplayName) {
			s.log.Info("Invalid display name", zap.String("display_name", req.DisplayName))
			return nil, status.Error(codes.InvalidArgument, "invalid display name")
		}

		s.log.Error("Failed to set display name", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to set display name")
	}

	return &profilepb.SetDisplayNameResponse{}, nil
}
