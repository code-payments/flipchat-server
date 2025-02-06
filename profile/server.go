package profile

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	profilepb "github.com/code-payments/flipchat-protobuf-api/generated/go/profile/v1"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/social/x"
)

type Server struct {
	log      *zap.Logger
	accounts account.Store
	profiles Store
	xClient  *x.Client
	authz    auth.Authorizer

	profilepb.UnimplementedProfileServer
}

func NewServer(log *zap.Logger, accounts account.Store, profiles Store, xClient *x.Client, authz auth.Authorizer) *Server {
	return &Server{
		log:      log,
		accounts: accounts,
		profiles: profiles,
		xClient:  xClient,
		authz:    authz,
	}
}

func (s *Server) GetProfile(ctx context.Context, req *profilepb.GetProfileRequest) (*profilepb.GetProfileResponse, error) {
	log := s.log.With(
		zap.String("user_id", model.UserIDString(req.UserId)),
	)

	profile, err := s.profiles.GetProfile(ctx, req.UserId)
	if errors.Is(err, ErrNotFound) {
		return &profilepb.GetProfileResponse{Result: profilepb.GetProfileResponse_NOT_FOUND}, nil
	} else if err != nil {
		log.Warn("Failed to get profile", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get profile")
	}

	return &profilepb.GetProfileResponse{UserProfile: profile}, nil
}

func (s *Server) SetDisplayName(ctx context.Context, req *profilepb.SetDisplayNameRequest) (*profilepb.SetDisplayNameResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("display_name", req.DisplayName),
	)

	isRegistered, err := s.accounts.IsRegistered(ctx, userID)
	if err != nil {
		log.Info("Failed to get registration flag")
		return nil, status.Errorf(codes.Internal, "failed to get registration flag")
	} else if !isRegistered {
		return &profilepb.SetDisplayNameResponse{Result: profilepb.SetDisplayNameResponse_DENIED}, nil
	}

	if err := s.profiles.SetDisplayName(ctx, userID, req.DisplayName); err != nil {
		if errors.Is(err, ErrInvalidDisplayName) {
			log.Info("Invalid display name")
			return nil, status.Error(codes.InvalidArgument, "invalid display name")
		}

		s.log.Warn("Failed to set display name", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to set display name")
	}

	return &profilepb.SetDisplayNameResponse{}, nil
}

func (s *Server) LinkXAccount(ctx context.Context, req *profilepb.LinkXAccountRequest) (*profilepb.LinkXAccountResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(zap.String("user_id", model.UserIDString(userID)))

	isRegistered, err := s.accounts.IsRegistered(ctx, userID)
	if err != nil {
		log.Info("Failed to get registration flag")
		return nil, status.Errorf(codes.Internal, "failed to get registration flag")
	} else if !isRegistered {
		return &profilepb.LinkXAccountResponse{Result: profilepb.LinkXAccountResponse_DENIED}, nil
	}

	xUser, err := s.xClient.GetMyUser(ctx, req.AccessToken)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "http status code: 403") {
			return &profilepb.LinkXAccountResponse{Result: profilepb.LinkXAccountResponse_INVALID_ACCESS_TOKEN}, nil
		}

		log.Warn("Failed to get user from x", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get user from x")
	}

	protoXUser := xUser.ToProto()

	if err := protoXUser.Validate(); err != nil {
		log.Warn("Failed to validate proto x profile")
		return nil, status.Error(codes.Internal, "failed to validate proto x profile")
	}

	err = s.profiles.LinkXAccount(ctx, userID, protoXUser, req.AccessToken)
	switch err {
	case nil:
	case ErrExistingSocialLink:
		return &profilepb.LinkXAccountResponse{Result: profilepb.LinkXAccountResponse_EXISTING_LINK}, nil
	default:
		log.Warn("failed to link x account", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to link x account")
	}

	return &profilepb.LinkXAccountResponse{XProfile: protoXUser}, nil
}
