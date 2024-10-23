package profile

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	profilepb "github.com/code-payments/flipchat-protobuf-api/generated/go/profile/v1"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/testutil"
)

func TestServer(t *testing.T) {
	accountStore := account.NewInMemory()
	authz := account.NewAuthorizer(accountStore, auth.NewKeyPairAuthenticator())

	serv := NewServer(zap.Must(zap.NewDevelopment()), NewInMemory(), authz)
	cc := testutil.RunGRPCServer(t, testutil.WithService(func(s *grpc.Server) {
		profilepb.RegisterProfileServer(s, serv)
	}))

	client := profilepb.NewProfileClient(cc)
	userID := account.MustGenerateUserID()
	keyPair := account.MustGenerateKeyPair()

	t.Run("No User", func(t *testing.T) {
		get, err := client.GetProfile(context.Background(), &profilepb.GetProfileRequest{
			UserId: userID,
		})
		require.NoError(t, err)
		require.Equal(t, profilepb.GetProfileResponse_NOT_FOUND, get.Result)
		require.Nil(t, get.UserProfile)

		req := &profilepb.SetDisplayNameRequest{
			UserId:      userID,
			DisplayName: "my name",
		}
		require.NoError(t, keyPair.Auth(req, &req.Auth))
		_, err = client.SetDisplayName(context.Background(), req)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("Allowed", func(t *testing.T) {
		_, _ = accountStore.Bind(context.Background(), userID, keyPair.Proto())

		// Binding of a user isn't sufficient, a profile must be set!
		get, err := client.GetProfile(context.Background(), &profilepb.GetProfileRequest{
			UserId: userID,
		})
		require.NoError(t, err)
		require.Equal(t, profilepb.GetProfileResponse_NOT_FOUND, get.Result)
		require.Nil(t, get.UserProfile)

		req := &profilepb.SetDisplayNameRequest{
			UserId:      userID,
			DisplayName: "my name",
		}
		require.NoError(t, keyPair.Auth(req, &req.Auth))
		_, err = client.SetDisplayName(context.Background(), req)
		require.NoError(t, err)

		get, err = client.GetProfile(context.Background(), &profilepb.GetProfileRequest{
			UserId: userID,
		})
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&profilepb.UserProfile{DisplayName: "my name"}, get.UserProfile))
	})
}
