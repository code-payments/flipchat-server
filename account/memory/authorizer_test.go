package memory

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountpb "github.com/code-payments/flipchat-protobuf-api/generated/go/account/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"
)

func TestAuthorizer(t *testing.T) {
	log := zap.Must(zap.NewDevelopment())
	store := NewInMemory()
	authn := auth.NewKeyPairAuthenticator()

	authz := account.NewAuthorizer(log, store, authn)

	userID := model.MustGenerateUserID()
	signer := model.MustGenerateKeyPair()

	t.Run("UserNotFound", func(t *testing.T) {
		newKeyPair := model.MustGenerateKeyPair()
		req := &accountpb.AuthorizePublicKeyRequest{
			UserId:    model.MustGenerateUserID(),
			PublicKey: newKeyPair.Proto(),
			Signature: nil,
			Auth:      nil,
		}
		require.NoError(t, newKeyPair.Sign(req, &req.Signature))
		require.NoError(t, signer.Auth(req, &req.Auth))

		_, err := authz.Authorize(context.Background(), req, &req.Auth)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.NotNil(t, req.Auth)
	})

	t.Run("Authorized", func(t *testing.T) {
		_, err := store.Bind(context.Background(), userID, signer.Proto())
		require.NoError(t, err)

		newKeyPair := model.MustGenerateKeyPair()
		req := &accountpb.AuthorizePublicKeyRequest{
			UserId:    userID,
			PublicKey: newKeyPair.Proto(),
			Signature: nil,
			Auth:      nil,
		}
		require.NoError(t, newKeyPair.Sign(req, &req.Signature))
		require.NoError(t, signer.Auth(req, &req.Auth))

		actual, err := authz.Authorize(context.Background(), req, &req.Auth)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(userID, actual))
	})

	t.Run("Unauthenticated - Missing", func(t *testing.T) {
		newKeyPair := model.MustGenerateKeyPair()
		req := &accountpb.AuthorizePublicKeyRequest{
			UserId:    userID,
			PublicKey: newKeyPair.Proto(),
			Signature: nil,
			Auth:      nil,
		}
		require.NoError(t, newKeyPair.Sign(req, &req.Signature))

		_, err := authz.Authorize(context.Background(), req, &req.Auth)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("Unauthenticated - Invalid", func(t *testing.T) {
		newKeyPair := model.MustGenerateKeyPair()
		req := &accountpb.AuthorizePublicKeyRequest{
			UserId:    userID,
			PublicKey: newKeyPair.Proto(),
			Signature: nil,
			Auth: &commonpb.Auth{
				Kind: &commonpb.Auth_KeyPair_{
					KeyPair: &commonpb.Auth_KeyPair{
						PubKey:    &commonpb.PublicKey{Value: bytes.Repeat([]byte{0}, 32)},
						Signature: &commonpb.Signature{Value: bytes.Repeat([]byte{0}, 64)},
					},
				},
			},
		}
		require.NoError(t, newKeyPair.Sign(req, &req.Signature))

		_, err := authz.Authorize(context.Background(), req, &req.Auth)
		require.Equal(t, codes.Unauthenticated, status.Code(err))
		require.NotNil(t, req.Auth)
	})
}
