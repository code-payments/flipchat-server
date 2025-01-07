package iap

import (
	"bytes"
	"context"
	"encoding/base64"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	iappb "github.com/code-payments/flipchat-protobuf-api/generated/go/iap/v1"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/model"
)

type Server struct {
	log            *zap.Logger
	authz          auth.Authorizer
	accounts       account.Store
	iaps           Store
	appleVerifier  Verifier
	googleVerifier Verifier

	iappb.UnimplementedIapServer
}

func NewServer(
	log *zap.Logger,
	authz auth.Authorizer,
	accounts account.Store,
	iaps Store,
	appleVerifier Verifier,
	googleVerifier Verifier,
) *Server {
	return &Server{
		log:            log,
		authz:          authz,
		accounts:       accounts,
		iaps:           iaps,
		appleVerifier:  appleVerifier,
		googleVerifier: googleVerifier,
	}
}

// todo: DB transaction for all calls
// todo: eventually we'll need to distinguish what was purchased
func (s *Server) OnPurchaseCompleted(ctx context.Context, req *iappb.OnPurchaseCompletedRequest) (*iappb.OnPurchaseCompletedResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	var verifier Verifier
	switch req.Platform {
	case commonpb.Platform_APPLE:
		verifier = s.appleVerifier
	case commonpb.Platform_GOOGLE:
		verifier = s.googleVerifier
	default:
		return &iappb.OnPurchaseCompletedResponse{Result: iappb.OnPurchaseCompletedResponse_DENIED}, nil
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("platform", req.Platform.String()),
		zap.String("receipt", req.Receipt.Value),
	)

	log.Debug("Got a receipt")

	isVerified, err := verifier.VerifyReceipt(ctx, req.Receipt.Value)
	if err != nil {
		log.Warn("Failed to verify receipt", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to verify receipt")
	} else if !isVerified {
		log.Warn("Receipt failed validation")
		return &iappb.OnPurchaseCompletedResponse{Result: iappb.OnPurchaseCompletedResponse_INVALID_RECEIPT}, nil
	}

	receiptID, err := verifier.GetReceiptIdentifier(ctx, req.Receipt.Value)
	if err != nil {
		log.Warn("Failed to get receipt ID", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get receipt ID")
	}

	log = s.log.With(
		zap.String("receipt_id", base64.StdEncoding.EncodeToString(receiptID)),
	)

	// Note: purchase is always assumed to be fulfilled
	purchase, err := s.iaps.GetPurchase(ctx, receiptID)
	if err == nil {
		if bytes.Equal(userID.Value, purchase.User.Value) {
			// Purchase is already fulfilled for this user, so it's a no-op. Return success
			return &iappb.OnPurchaseCompletedResponse{}, nil
		}
		log.Warn("Denying attempt to use an already fulfilled receipt")
		return &iappb.OnPurchaseCompletedResponse{Result: iappb.OnPurchaseCompletedResponse_DENIED}, nil
	} else if err != ErrNotFound {
		log.Warn("Failed to check existing purchase", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to check existing purchase")
	}

	err = s.accounts.SetRegistrationFlag(ctx, userID, true)
	if err != nil {
		log.Warn("Failed to set registration flag", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to set registration flag")
	}

	err = s.iaps.CreatePurchase(ctx, &Purchase{
		ReceiptID: receiptID,
		Platform:  req.Platform,
		User:      userID,
		Product:   ProductCreateAccount,
		State:     StateFulfilled,
		CreatedAt: time.Now(),
	})
	if err != nil {
		log.Warn("Failed to create purchase", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create purchase")
	}

	return &iappb.OnPurchaseCompletedResponse{}, nil
}
