package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	iappb "github.com/code-payments/flipchat-protobuf-api/generated/go/iap/v1"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/iap"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"
)

// RunServerTests runs a set of tests against the iap.Server.
func RunServerTests(t *testing.T, s account.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s account.Store){
		testOnPurchaseCompleted,
	} {
		tf(t, s)
		teardown()
	}
}

func testOnPurchaseCompleted(t *testing.T, store account.Store) {
	log := zap.Must(zap.NewDevelopment())
	authn := auth.NewKeyPairAuthenticator()
	authz := account.NewAuthorizer(log, store, authn)
	server := iap.NewServer(log, authz)

	signer := model.MustGenerateKeyPair()

	t.Run("UserNotFound", func(t *testing.T) {
		// Here we simulate a call to OnPurchaseCompleted using a key that's not
		// bound in the store.
		req := &iappb.OnPurchaseCompletedRequest{
			Platform: commonpb.Platform_APPLE,
			Receipt:  &iappb.Receipt{}, // A dummy receipt for testing
			Auth:     nil,
		}

		// Authenticate the request. Since `signer`'s key is not bound to any user,
		// we expect the authorize call inside OnPurchaseCompleted to fail.
		require.NoError(t, signer.Auth(req, &req.Auth))

		_, err := server.OnPurchaseCompleted(context.Background(), req)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.NotNil(t, req.Auth)
	})

	t.Run("Authorized", func(t *testing.T) {
		// Bind the user's key in the store so that `authz` can recognize them.
		userID := model.MustGenerateUserID()
		_, err := store.Bind(context.Background(), userID, signer.Proto())
		require.NoError(t, err)

		req := &iappb.OnPurchaseCompletedRequest{
			Platform: commonpb.Platform_GOOGLE,
			Receipt:  &iappb.Receipt{}, // A dummy receipt for testing
			Auth:     nil,
		}

		// Now that the user is bound, `authz` should recognize them and authorize the request.
		require.NoError(t, signer.Auth(req, &req.Auth))

		resp, err := server.OnPurchaseCompleted(context.Background(), req)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&iappb.OnPurchaseCompletedResponse{}, resp))
	})
}
