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
func RunServerTests(t *testing.T, accounts account.Store, iaps iap.Store, verifer iap.Verifier, validReceiptFunc func(msg string) string, teardown func()) {
	for _, tf := range []func(t *testing.T, accountStore account.Store, iaps iap.Store, verifer iap.Verifier, validReceiptFunc func(msg string) string){
		testOnPurchaseCompleted,
	} {
		tf(t, accounts, iaps, verifer, validReceiptFunc)
		teardown()
	}
}

func testOnPurchaseCompleted(t *testing.T, accounts account.Store, iaps iap.Store, verifer iap.Verifier, validReceiptFunc func(msg string) string) {
	log := zap.Must(zap.NewDevelopment())
	authn := auth.NewKeyPairAuthenticator()
	authz := account.NewAuthorizer(log, accounts, authn)
	server := iap.NewServer(log, authz, accounts, iaps, verifer)

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

	t.Run("Valid Receipt", func(t *testing.T) {
		// Bind the user's key in the store so that `authz` can recognize them.
		userID := model.MustGenerateUserID()
		_, err := accounts.Bind(context.Background(), userID, signer.Proto())
		require.NoError(t, err)

		req := &iappb.OnPurchaseCompletedRequest{
			Platform: commonpb.Platform_GOOGLE,
			Receipt:  &iappb.Receipt{Value: validReceiptFunc("create account")}, // A valid dummy receipt for testing
			Auth:     nil,
		}

		// Now that the user is bound, `authz` should recognize them and authorize the request.
		require.NoError(t, signer.Auth(req, &req.Auth))

		resp, err := server.OnPurchaseCompleted(context.Background(), req)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&iappb.OnPurchaseCompletedResponse{}, resp))

		isRegistered, err := accounts.IsRegistered(context.Background(), userID)
		require.NoError(t, err)
		require.True(t, isRegistered)

		purchase, err := iaps.GetPurchase(context.Background(), req.Receipt.Value)
		require.NoError(t, err)
		require.Equal(t, req.Receipt.Value, purchase.Receipt)
		require.Equal(t, req.Platform, purchase.Platform)
		require.NoError(t, protoutil.ProtoEqualError(userID, purchase.User))
		require.Equal(t, iap.ProductCreateAccount, purchase.Product)
		require.Equal(t, iap.StateFulfilled, purchase.State)

		t.Run("Use existing receipt", func(t *testing.T) {
			userID2 := model.MustGenerateUserID()
			signer2 := model.MustGenerateKeyPair()
			_, err := accounts.Bind(context.Background(), userID2, signer2.Proto())
			require.NoError(t, err)

			require.NoError(t, signer.Auth(req, &req.Auth))

			resp, err := server.OnPurchaseCompleted(context.Background(), req)
			require.NoError(t, err)
			require.NoError(t, protoutil.ProtoEqualError(&iappb.OnPurchaseCompletedResponse{Result: iappb.OnPurchaseCompletedResponse_INVALID_RECEIPT}, resp))

			isRegistered, err := accounts.IsRegistered(context.Background(), userID2)
			require.NoError(t, err)
			require.False(t, isRegistered)

			purchase, err := iaps.GetPurchase(context.Background(), req.Receipt.Value)
			require.NoError(t, err)
			require.NoError(t, protoutil.ProtoEqualError(userID, purchase.User))
		})
	})

	t.Run("Invalid Receipt", func(t *testing.T) {
		// Bind the user's key in the store so that `authz` can recognize them.
		userID := model.MustGenerateUserID()
		_, err := accounts.Bind(context.Background(), userID, signer.Proto())
		require.NoError(t, err)

		req := &iappb.OnPurchaseCompletedRequest{
			Platform: commonpb.Platform_GOOGLE,
			Receipt:  &iappb.Receipt{Value: "invalid"}, // An invalid dummy receipt for testing
			Auth:     nil,
		}

		// Now that the user is bound, `authz` should recognize them and authorize the request.
		require.NoError(t, signer.Auth(req, &req.Auth))

		resp, err := server.OnPurchaseCompleted(context.Background(), req)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&iappb.OnPurchaseCompletedResponse{Result: iappb.OnPurchaseCompletedResponse_INVALID_RECEIPT}, resp))

		isRegistered, err := accounts.IsRegistered(context.Background(), userID)
		require.NoError(t, err)
		require.False(t, isRegistered)

		_, err = iaps.GetPurchase(context.Background(), req.Receipt.Value)
		require.Equal(t, iap.ErrNotFound, err)
	})
}
