package iap

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	iappb "github.com/code-payments/flipchat-protobuf-api/generated/go/iap/v1"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/model"
)

type Server struct {
	log      *zap.Logger
	authz    auth.Authorizer
	accounts account.Store
	verifier Verifier

	iappb.UnimplementedIapServer
}

func NewServer(
	log *zap.Logger,
	authz auth.Authorizer,
	accounts account.Store,
	verifier Verifier,
) *Server {
	return &Server{
		log:      log,
		authz:    authz,
		accounts: accounts,
		verifier: verifier,
	}
}

// todo: eventually we'll need to distinguish what was purchased
func (s *Server) OnPurchaseCompleted(ctx context.Context, req *iappb.OnPurchaseCompletedRequest) (*iappb.OnPurchaseCompletedResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("platform", req.Platform.String()),
		zap.String("receipt", req.Receipt.Value),
	)

	// todo: check if the receipt was already consumed

	isVerified, err := s.verifier.VerifyReceipt(ctx, req.Receipt.Value)
	if err != nil {
		log.Warn("Failed to verify receipt", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to set registration flag")
	} else if !isVerified {
		return &iappb.OnPurchaseCompletedResponse{Result: iappb.OnPurchaseCompletedResponse_INVALID_RECEIPT}, nil
	}

	err = s.accounts.SetRegistrationFlag(ctx, userID, true)
	if err != nil {
		log.Warn("Failed to set registration flag", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to set registration flag")
	}

	// todo: mark the receipt as being consumed

	return &iappb.OnPurchaseCompletedResponse{}, nil
}
