package account

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/auth"
)

type Authorizer struct {
	store Store
	authn auth.Authenticator
}

func NewAuthorizer(store Store, authn auth.Authenticator) *Authorizer {
	return &Authorizer{
		store: store,
		authn: authn,
	}
}

func (a *Authorizer) Authorize(ctx context.Context, m proto.Message, userId *commonpb.UserId, authField **commonpb.Auth) error {
	authMessage := *authField
	*authField = nil

	defer func() {
		*authField = authMessage
	}()

	if err := a.authn.Verify(ctx, m, authMessage); err != nil {
		return err
	}

	ok, err := a.store.IsAuthorized(ctx, userId, authMessage.GetKeyPair().GetPubKey())
	if err != nil {
		return status.Error(codes.Internal, "failed to authorize")
	}

	if !ok {
		return status.Error(codes.PermissionDenied, "permission denied")
	}

	return nil
}
