package account

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/auth"
)

type Authorizer struct {
	log   *zap.Logger
	store Store
	authn auth.Authenticator
}

func NewAuthorizer(log *zap.Logger, store Store, authn auth.Authenticator) *Authorizer {
	return &Authorizer{
		log:   log,
		store: store,
		authn: authn,
	}
}

func (a *Authorizer) AuthorizeWithUser(ctx context.Context, m proto.Message, userId *commonpb.UserId, authField **commonpb.Auth) error {
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
func (a *Authorizer) Authorize(ctx context.Context, m proto.Message, authField **commonpb.Auth) (*commonpb.UserId, error) {
	authMessage := *authField
	*authField = nil

	defer func() {
		*authField = authMessage
	}()

	if err := a.authn.Verify(ctx, m, authMessage); err != nil {
		return nil, err
	}

	userID, err := a.store.GetUserId(ctx, authMessage.GetKeyPair().GetPubKey())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	} else if err != nil {
		return nil, status.Error(codes.Internal, "failed to verify auth")
	}

	return userID, nil
}
