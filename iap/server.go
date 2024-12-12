package iap

import (
	"context"

	"go.uber.org/zap"

	iappb "github.com/code-payments/flipchat-protobuf-api/generated/go/iap/v1"

	"github.com/code-payments/flipchat-server/auth"
)

type Server struct {
	log   *zap.Logger
	authz auth.Authorizer

	iappb.UnimplementedIapServer
}

func NewServer(
	log *zap.Logger,
	authz auth.Authorizer,
) *Server {
	return &Server{
		log:   log,
		authz: authz,
	}
}

func (s *Server) OnPurchaseCompleted(ctx context.Context, req *iappb.OnPurchaseCompletedRequest) (*iappb.OnPurchaseCompletedResponse, error) {
	_, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	return &iappb.OnPurchaseCompletedResponse{}, nil
}
