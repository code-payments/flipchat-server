package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	accountpb "github.com/code-payments/flipchat-protobuf-api/generated/go/account/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codedata "github.com/code-payments/code-server/pkg/code/data"
	codetestutil "github.com/code-payments/code-server/pkg/testutil"

	"github.com/code-payments/flipchat-server/account"

	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/profile"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/testutil"
)

func TestServer(t *testing.T) {
	store := NewInMemory()
	profiles := profile.NewInMemory()
	codeStores := codedata.NewTestDataProvider()

	server := account.NewServer(
		zap.Must(zap.NewDevelopment()),
		store,
		profiles,
		auth.NewKeyPairAuthenticator(),
	)

	cc := testutil.RunGRPCServer(t, testutil.WithService(func(s *grpc.Server) {
		accountpb.RegisterAccountServer(s, server)
	}))

	ctx := context.Background()
	client := accountpb.NewAccountClient(cc)

	codetestutil.SetupRandomSubsidizer(t, codeStores)

	var keys []model.KeyPair
	var userId *commonpb.UserId

	t.Run("Register", func(t *testing.T) {
		keys = append(keys, model.MustGenerateKeyPair())
		req := &accountpb.RegisterRequest{
			PublicKey:   keys[0].Proto(),
			DisplayName: "hello!",
		}
		require.NoError(t, keys[0].Sign(req, &req.Signature))

		for range 2 {
			resp, err := client.Register(ctx, req)
			require.NoError(t, err)
			require.Equal(t, accountpb.RegisterResponse_OK, resp.Result)
			require.NotNil(t, resp.UserId)

			if userId == nil {
				userId = resp.UserId
			} else {
				require.NoError(t, protoutil.ProtoEqualError(userId, resp.UserId))
			}

			p, err := profiles.GetProfile(ctx, resp.UserId)
			require.NoError(t, err)
			require.Equal(t, "hello!", p.GetDisplayName())
		}
	})

	t.Run("AuthorizePublicKey", func(t *testing.T) {
		for i := 1; i < 10; i++ {
			newPair := model.MustGenerateKeyPair()
			req := &accountpb.AuthorizePublicKeyRequest{
				UserId:    userId,
				PublicKey: newPair.Proto(),
			}
			require.NoError(t, newPair.Sign(req, &req.Signature))
			require.NoError(t, keys[i-1].Auth(req, &req.Auth))

			resp, err := client.AuthorizePublicKey(ctx, req)
			require.NoError(t, err)

			require.NoError(t, err)
			require.Equal(t, accountpb.AuthorizePublicKeyResponse_OK, resp.Result)

			keys = append(keys, newPair)

			expected := make([]*commonpb.PublicKey, len(keys))
			for k := range keys {
				expected[k] = keys[k].Proto()
			}

			actual, err := store.GetPubKeys(ctx, userId)
			require.NoError(t, err)

			require.NoError(t, protoutil.SliceEqualError(expected, actual))
		}
	})

	t.Run("Login", func(t *testing.T) {
		for _, key := range keys {
			req := &accountpb.LoginRequest{
				Timestamp: timestamppb.Now(),
			}
			require.NoError(t, key.Auth(req, &req.Auth))

			resp, err := client.Login(ctx, req)
			require.NoError(t, err)
			require.Equal(t, accountpb.LoginResponse_OK, resp.Result)
			require.NotNil(t, resp.UserId)
			require.NoError(t, protoutil.ProtoEqualError(userId, resp.UserId))
		}
	})

	t.Run("Cannot Remove Another Accounts Key", func(t *testing.T) {
		other := model.MustGenerateKeyPair()
		register := &accountpb.RegisterRequest{
			PublicKey:   other.Proto(),
			DisplayName: "hello!",
		}
		require.NoError(t, other.Sign(register, &register.Signature))

		registerResp, err := client.Register(ctx, register)
		require.NoError(t, err)
		require.Equal(t, accountpb.RegisterResponse_OK, registerResp.Result)

		req := &accountpb.RevokePublicKeyRequest{
			UserId:    registerResp.UserId,
			PublicKey: other.Proto(),
		}
		require.NoError(t, keys[0].Auth(req, &req.Auth))

		// We expect OK, since the signer owns the requested account.
		resp, err := client.RevokePublicKey(ctx, req)
		require.NoError(t, err)
		require.Equal(t, accountpb.RevokePublicKeyResponse_DENIED, resp.Result)
	})

	t.Run("Cannot Remove Another Accounts Key Indirectly", func(t *testing.T) {
		other := model.MustGenerateKeyPair()
		register := &accountpb.RegisterRequest{
			PublicKey:   other.Proto(),
			DisplayName: "hello!",
		}
		require.NoError(t, other.Sign(register, &register.Signature))

		registerResp, err := client.Register(ctx, register)
		require.NoError(t, err)
		require.Equal(t, accountpb.RegisterResponse_OK, registerResp.Result)

		req := &accountpb.RevokePublicKeyRequest{
			UserId:    userId,
			PublicKey: other.Proto(),
		}
		require.NoError(t, keys[0].Auth(req, &req.Auth))

		// We expect OK, since the signer owns the requested account.
		resp, err := client.RevokePublicKey(ctx, req)
		require.NoError(t, err)
		require.Equal(t, accountpb.RevokePublicKeyResponse_OK, resp.Result)

		// However, it should have had zero effect.
		login := &accountpb.LoginRequest{Timestamp: timestamppb.Now()}
		require.NoError(t, other.Auth(login, &login.Auth))

		loginResp, err := client.Login(ctx, login)
		require.NoError(t, err)
		require.Equal(t, accountpb.LoginResponse_OK, loginResp.Result)
	})

	t.Run("RevokePublicKey - Other", func(t *testing.T) {
		remove := keys[len(keys)-1]
		keys = keys[:len(keys)-1]

		req := &accountpb.RevokePublicKeyRequest{
			UserId:    userId,
			PublicKey: remove.Proto(),
		}
		require.NoError(t, keys[0].Auth(req, &req.Auth))

		// Technically any of the keys should be able to auth here.
		//
		// We do it twice for idempotency, which is allowed since the
		// revoker is still a signing authority on the account.
		for range 2 {
			resp, err := client.RevokePublicKey(ctx, req)
			require.NoError(t, err)
			require.Equal(t, accountpb.RevokePublicKeyResponse_OK, resp.Result)

			expected := make([]*commonpb.PublicKey, len(keys))
			for k := range keys {
				expected[k] = keys[k].Proto()
			}

			actual, err := store.GetPubKeys(ctx, userId)
			require.NoError(t, protoutil.SliceEqualError(expected, actual))

			login := &accountpb.LoginRequest{Timestamp: timestamppb.Now()}
			require.NoError(t, remove.Auth(login, &login.Auth))

			loginResp, err := client.Login(ctx, login)
			require.NoError(t, err)
			require.Equal(t, accountpb.LoginResponse_DENIED, loginResp.Result)
		}
	})

	t.Run("RevokePublicKey - Self", func(t *testing.T) {
		for len(keys) > 1 {
			key := keys[len(keys)-1]
			keys = keys[:len(keys)-1]

			req := &accountpb.RevokePublicKeyRequest{
				UserId:    userId,
				PublicKey: key.Proto(),
			}
			require.NoError(t, key.Auth(req, &req.Auth))

			resp, err := client.RevokePublicKey(ctx, req)
			require.NoError(t, err)
			require.Equal(t, accountpb.RevokePublicKeyResponse_OK, resp.Result)

			expected := make([]*commonpb.PublicKey, len(keys))
			for k := range keys {
				expected[k] = keys[k].Proto()
			}

			actual, err := store.GetPubKeys(ctx, userId)
			require.NoError(t, protoutil.SliceEqualError(expected, actual))

			login := &accountpb.LoginRequest{Timestamp: timestamppb.Now()}
			require.NoError(t, key.Auth(login, &login.Auth))

			loginResp, err := client.Login(ctx, login)
			require.NoError(t, err)
			require.Equal(t, accountpb.LoginResponse_DENIED, loginResp.Result)

			resp, err = client.RevokePublicKey(ctx, req)
			require.NoError(t, err)
			require.Equal(t, accountpb.RevokePublicKeyResponse_DENIED, resp.Result)
		}
	})

	t.Run("Cannot remove last", func(t *testing.T) {
		key := keys[0]
		req := &accountpb.RevokePublicKeyRequest{
			UserId:    userId,
			PublicKey: key.Proto(),
		}
		require.NoError(t, key.Auth(req, &req.Auth))

		resp, err := client.RevokePublicKey(ctx, req)
		require.NoError(t, err)
		require.Equal(t, accountpb.RevokePublicKeyResponse_LAST_PUB_KEY, resp.Result)

		actual, err := store.GetPubKeys(ctx, userId)
		require.NoError(t, err)
		require.Len(t, actual, 1)
	})

	t.Run("GetPaymentDestination", func(t *testing.T) {
		ownerAccount, err := codecommon.NewAccountFromPublicKeyBytes(keys[0].Public())
		require.NoError(t, err)
		expected, err := ownerAccount.ToTimelockVault(codecommon.CodeVmAccount, codecommon.KinMintAccount)
		require.NoError(t, err)

		req := &accountpb.GetPaymentDestinationRequest{
			UserId: userId,
		}

		resp, err := client.GetPaymentDestination(ctx, req)
		require.NoError(t, err)
		require.Equal(t, accountpb.GetPaymentDestinationResponse_OK, resp.Result)
		require.Equal(t, expected.PublicKey().ToBytes(), resp.PaymentDestination.Value)
	})

}
