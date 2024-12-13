package account

import (
	"bytes"
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountpb "github.com/code-payments/flipchat-protobuf-api/generated/go/account/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/flags"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/profile"
)

const loginWindow = 5 * time.Minute

type Server struct {
	log      *zap.Logger
	store    Store
	profiles profile.Store
	verifier auth.Authenticator

	accountpb.UnimplementedAccountServer
}

func NewServer(log *zap.Logger, store Store, profiles profile.Store, verifier auth.Authenticator) *Server {
	return &Server{
		log:      log,
		store:    store,
		profiles: profiles,
		verifier: verifier,
	}
}

func (s *Server) Register(ctx context.Context, req *accountpb.RegisterRequest) (*accountpb.RegisterResponse, error) {
	verify := &accountpb.RegisterRequest{
		PublicKey:   req.PublicKey,
		DisplayName: req.DisplayName,
	}
	err := s.verifier.Verify(ctx, verify, &commonpb.Auth{
		Kind: &commonpb.Auth_KeyPair_{
			KeyPair: &commonpb.Auth_KeyPair{
				PubKey:    req.PublicKey,
				Signature: req.Signature,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	userID, err := model.GenerateUserId()
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to generate user id")
	}

	prev, err := s.store.Bind(ctx, userID, req.PublicKey)
	if err != nil {
		return nil, status.Error(codes.Internal, "")
	}

	if len(req.DisplayName) > 0 {
		if err = s.profiles.SetDisplayName(ctx, userID, req.DisplayName); err != nil {
			return nil, status.Error(codes.Internal, "failed to set display name")
		}
	}

	return &accountpb.RegisterResponse{
		UserId: prev,
	}, nil
}

func (s *Server) Login(ctx context.Context, req *accountpb.LoginRequest) (*accountpb.LoginResponse, error) {
	t := req.Timestamp.AsTime()
	if t.After(time.Now()) {
		return &accountpb.LoginResponse{Result: accountpb.LoginResponse_INVALID_TIMESTAMP}, nil
	} else if t.Before(time.Now().Add(-loginWindow)) {
		return &accountpb.LoginResponse{Result: accountpb.LoginResponse_INVALID_TIMESTAMP}, nil
	}

	a := req.Auth
	req.Auth = nil
	if err := s.verifier.Verify(ctx, req, a); err != nil {
		if status.Code(err) == codes.Unauthenticated {
			return &accountpb.LoginResponse{Result: accountpb.LoginResponse_DENIED}, nil
		}

		return nil, err
	}

	keyPair := a.GetKeyPair()
	if keyPair == nil {
		return nil, status.Error(codes.InvalidArgument, "missing keypair")
	}
	if err := keyPair.Validate(); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid keypair: %v", err)
	}

	userID, err := s.store.GetUserId(ctx, keyPair.GetPubKey())
	if errors.Is(err, ErrNotFound) {
		return &accountpb.LoginResponse{Result: accountpb.LoginResponse_DENIED}, nil
	} else if err != nil {
		return nil, status.Error(codes.Internal, "")
	}

	return &accountpb.LoginResponse{Result: accountpb.LoginResponse_OK, UserId: userID}, nil
}

func (s *Server) AuthorizePublicKey(ctx context.Context, req *accountpb.AuthorizePublicKeyRequest) (*accountpb.AuthorizePublicKeyResponse, error) {
	pubKeys, err := s.store.GetPubKeys(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Unimplemented, "failed to lookup user")
	}
	if len(pubKeys) == 0 {
		return &accountpb.AuthorizePublicKeyResponse{Result: accountpb.AuthorizePublicKeyResponse_DENIED}, nil
	}

	authorizingKey := req.Auth.GetKeyPair()
	if authorizingKey == nil {
		return nil, status.Error(codes.InvalidArgument, "")
	}

	var found bool
	for _, key := range pubKeys {
		if bytes.Equal(key.Value, req.PublicKey.Value) {
			return &accountpb.AuthorizePublicKeyResponse{}, nil
		}

		if bytes.Equal(key.Value, authorizingKey.PubKey.Value) {
			found = true
			break
		}
	}

	if !found {
		return &accountpb.AuthorizePublicKeyResponse{Result: accountpb.AuthorizePublicKeyResponse_DENIED}, nil
	}

	prev, err := s.store.Bind(ctx, req.UserId, req.PublicKey)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to authorize public key")
	}

	if !bytes.Equal(prev.Value, req.UserId.Value) {
		// warn
	}

	return &accountpb.AuthorizePublicKeyResponse{}, nil
}

func (s *Server) RevokePublicKey(ctx context.Context, req *accountpb.RevokePublicKeyRequest) (*accountpb.RevokePublicKeyResponse, error) {
	authorized, err := s.store.GetPubKeys(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get keys")
	}

	if len(authorized) == 0 {
		// Don't leak that the user does not exist.
		return &accountpb.RevokePublicKeyResponse{Result: accountpb.RevokePublicKeyResponse_DENIED}, nil
	}

	var signerAuthorized bool
	for _, key := range authorized {
		if bytes.Equal(key.Value, req.GetAuth().GetKeyPair().PubKey.Value) {
			signerAuthorized = true
			break
		}
	}

	if !signerAuthorized {
		return &accountpb.RevokePublicKeyResponse{Result: accountpb.RevokePublicKeyResponse_DENIED}, nil
	}

	if len(authorized) == 1 {
		// At this point, the signer owns the account, so it's ok to leak information
		return &accountpb.RevokePublicKeyResponse{Result: accountpb.RevokePublicKeyResponse_LAST_PUB_KEY}, nil
	}

	if err = s.store.RemoveKey(ctx, req.UserId, req.PublicKey); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to remove key")
	}

	return &accountpb.RevokePublicKeyResponse{}, nil
}

func (s *Server) GetPaymentDestination(ctx context.Context, req *accountpb.GetPaymentDestinationRequest) (*accountpb.GetPaymentDestinationResponse, error) {
	authorized, err := s.store.GetPubKeys(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get keys")
	}

	if len(authorized) == 0 {
		return &accountpb.GetPaymentDestinationResponse{Result: accountpb.GetPaymentDestinationResponse_NOT_FOUND}, nil
	}

	if len(authorized) != 1 {
		// todo: Handle multiple public keys. For now, each user gets one access key.
		//       We'll also be implementing some form of public declaration of a
		//       payment destination as well.
		return nil, status.Errorf(codes.Internal, "multiple public keys")
	}

	ownerAccount, err := codecommon.NewAccountFromPublicKeyBytes(authorized[0].Value)
	if err != nil {
		return nil, status.Error(codes.Internal, "invalid public key")
	}
	timelockAccount, err := ownerAccount.ToTimelockVault(codecommon.CodeVmAccount, codecommon.KinMintAccount)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to derive timelock vault")
	}

	return &accountpb.GetPaymentDestinationResponse{
		Result:             accountpb.GetPaymentDestinationResponse_OK,
		PaymentDestination: &commonpb.PublicKey{Value: timelockAccount.PublicKey().ToBytes()},
	}, nil
}

func (s *Server) GetUserFlags(ctx context.Context, req *accountpb.GetUserFlagsRequest) (*accountpb.GetUserFlagsResponse, error) {
	authorized, err := s.store.GetPubKeys(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get keys")
	}

	if len(authorized) == 0 {
		// Don't leak that the user does not exist.
		return &accountpb.GetUserFlagsResponse{Result: accountpb.GetUserFlagsResponse_DENIED}, nil
	}

	var signerAuthorized bool
	for _, key := range authorized {
		if bytes.Equal(key.Value, req.GetAuth().GetKeyPair().PubKey.Value) {
			signerAuthorized = true
			break
		}
	}

	if !signerAuthorized {
		return &accountpb.GetUserFlagsResponse{Result: accountpb.GetUserFlagsResponse_DENIED}, nil
	}

	isStaff, err := s.store.IsStaff(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get staff flag")
	}

	isRegistered, err := s.store.IsRegistered(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get registration flag")
	}

	return &accountpb.GetUserFlagsResponse{
		Result: accountpb.GetUserFlagsResponse_OK,
		UserFlags: &accountpb.UserFlags{
			IsStaff:             isStaff,
			IsRegisteredAccount: isRegistered,
			StartGroupFee:       &commonpb.PaymentAmount{Quarks: flags.StartGroupFee},
			FeeDestination:      &commonpb.PublicKey{Value: flags.FeeDestination.PublicKey().ToBytes()},
		},
	}, nil
}
