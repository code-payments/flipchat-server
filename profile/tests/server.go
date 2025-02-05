package tests

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
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/social/x"

	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/profile"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/testutil"
)

func RunServerTests(t *testing.T, accounts account.Store, profiles profile.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, accounts account.Store, profiles profile.Store){
		testServer,
	} {
		tf(t, accounts, profiles)
		teardown()
	}
}

func testServer(t *testing.T, accounts account.Store, profiles profile.Store) {
	authz := account.NewAuthorizer(zap.Must(zap.NewDevelopment()), accounts, auth.NewKeyPairAuthenticator())

	serv := profile.NewServer(zap.Must(zap.NewDevelopment()), accounts, profiles, x.NewClient(), authz)
	cc := testutil.RunGRPCServer(t, testutil.WithService(func(s *grpc.Server) {
		profilepb.RegisterProfileServer(s, serv)
	}))

	client := profilepb.NewProfileClient(cc)
	userID := model.MustGenerateUserID()
	keyPair := model.MustGenerateKeyPair()

	t.Run("No User", func(t *testing.T) {
		get, err := client.GetProfile(context.Background(), &profilepb.GetProfileRequest{
			UserId: userID,
		})
		require.NoError(t, err)
		require.Equal(t, profilepb.GetProfileResponse_NOT_FOUND, get.Result)
		require.Nil(t, get.UserProfile)

		req := &profilepb.SetDisplayNameRequest{
			DisplayName: "my name",
		}
		require.NoError(t, keyPair.Auth(req, &req.Auth))
		_, err = client.SetDisplayName(context.Background(), req)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("Registered user", func(t *testing.T) {
		_, err := accounts.Bind(context.Background(), userID, keyPair.Proto())
		require.NoError(t, err)
		require.NoError(t, accounts.SetRegistrationFlag(context.Background(), userID, true))

		// Binding of a user isn't sufficient, a profile must be set!
		get, err := client.GetProfile(context.Background(), &profilepb.GetProfileRequest{
			UserId: userID,
		})
		require.NoError(t, err)
		require.Equal(t, profilepb.GetProfileResponse_NOT_FOUND, get.Result)
		require.Nil(t, get.UserProfile)

		setDisplayName := &profilepb.SetDisplayNameRequest{
			DisplayName: "my name",
		}
		require.NoError(t, keyPair.Auth(setDisplayName, &setDisplayName.Auth))
		setDisplayNameResp, err := client.SetDisplayName(context.Background(), setDisplayName)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&profilepb.SetDisplayNameResponse{Result: profilepb.SetDisplayNameResponse_OK}, setDisplayNameResp))

		expected := &profilepb.UserProfile{DisplayName: "my name"}

		get, err = client.GetProfile(context.Background(), &profilepb.GetProfileRequest{
			UserId: userID,
		})
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(expected, get.UserProfile))

		xProfile := &profilepb.XProfile{
			Id:            "123",
			Username:      "registered_user",
			Name:          "registered name",
			Description:   "description",
			ProfilePicUrl: "url",
			VerifiedType:  profilepb.XProfile_BLUE,
			FollowerCount: 888,
		}
		// todo: Need mock X client to use the RPC
		require.NoError(t, profiles.LinkXAccount(context.Background(), userID, xProfile, "access_token"))

		expected.SocialProfiles = append(expected.SocialProfiles, &profilepb.SocialProfile{
			Type: &profilepb.SocialProfile_X{
				X: xProfile,
			},
		})
		get, err = client.GetProfile(context.Background(), &profilepb.GetProfileRequest{
			UserId: userID,
		})
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(expected, get.UserProfile))
	})

	t.Run("Unregistered user", func(t *testing.T) {
		userID2 := model.MustGenerateUserID()
		keypair2 := model.MustGenerateKeyPair()

		_, err := accounts.Bind(context.Background(), userID2, keypair2.Proto())
		require.NoError(t, err)
		require.NoError(t, accounts.SetRegistrationFlag(context.Background(), userID, false))

		setDisplayName := &profilepb.SetDisplayNameRequest{
			DisplayName: "my name",
		}
		require.NoError(t, keypair2.Auth(setDisplayName, &setDisplayName.Auth))
		setDisplayNameResp, err := client.SetDisplayName(context.Background(), setDisplayName)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&profilepb.SetDisplayNameResponse{Result: profilepb.SetDisplayNameResponse_DENIED}, setDisplayNameResp))

		linkXAccount := &profilepb.LinkXAccountRequest{
			AccessToken: "access_token",
		}
		require.NoError(t, keypair2.Auth(linkXAccount, &linkXAccount.Auth))
		linkXAccountResp, err := client.LinkXAccount(context.Background(), linkXAccount)
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&profilepb.LinkXAccountResponse{Result: profilepb.LinkXAccountResponse_DENIED}, linkXAccountResp))

		get, err := client.GetProfile(context.Background(), &profilepb.GetProfileRequest{
			UserId: userID2,
		})
		require.NoError(t, err)
		require.NoError(t, protoutil.ProtoEqualError(&profilepb.GetProfileResponse{Result: profilepb.GetProfileResponse_NOT_FOUND}, get))
	})
}
