package auth

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	codecommonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	auth2 "github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
)

// Authorizer authorizes an action for a UserId with the given auth.
//
// If the auth is authorized, it is also authenticated. Authorization is more expensive
// than authentication as lookups must be performed.
type Authorizer interface {
	Authorize(ctx context.Context, m proto.Message, userId *commonpb.UserId, authField **commonpb.Auth) error
}

// Authenticator authenticates a message with the provided auth.
//
// It is not usually sufficient to rely purely on authentication for permissions,
// as a lookup must be completed. However, if a message is not authentic, we can
// short circuit authorization.
//
// In general, users should use an Authorizer, and Authorizer's should use an
// Authenticator.
type Authenticator interface {
	Verify(ctx context.Context, m proto.Message, auth *commonpb.Auth) error
}

// NewKeyPairAuthenticator authenticates pub key based auth.
func NewKeyPairAuthenticator() Authenticator {
	return &authenticator{
		auth: auth2.NewRPCSignatureVerifier(nil),
	}
}

type authenticator struct {
	auth *auth2.RPCSignatureVerifier
}

func (v *authenticator) Verify(ctx context.Context, m proto.Message, auth *commonpb.Auth) error {
	keyPair := auth.GetKeyPair()
	if keyPair == nil {
		return status.Error(codes.InvalidArgument, "missing keypair")
	}

	if err := keyPair.Validate(); err != nil {
		return status.Error(codes.InvalidArgument, "invalid auth")
	}

	account, err := common.NewAccountFromPublicKeyBytes(keyPair.PubKey.Value)
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid pubkey")
	}

	return v.auth.Authenticate(ctx, account, m, &codecommonpb.Signature{Value: keyPair.Signature.Value})
}
