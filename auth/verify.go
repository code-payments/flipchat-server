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

type Verifier interface {
	Verify(ctx context.Context, m proto.Message, auth *commonpb.Auth) error
}

func NewVerifier() Verifier {
	return &verifier{
		auth: auth2.NewRPCSignatureVerifier(nil),
	}
}

type verifier struct {
	auth *auth2.RPCSignatureVerifier
}

func (v *verifier) Verify(ctx context.Context, m proto.Message, auth *commonpb.Auth) error {
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
