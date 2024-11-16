package push

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	pushpb "github.com/code-payments/flipchat-protobuf-api/generated/go/push/v1"

	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/model"
)

func TestServer_AddToken(t *testing.T) {
	ctx := context.Background()
	log := zaptest.NewLogger(t)
	store := NewMemory()
	authz := auth.NewStaticAuthorizer()
	server := NewServer(log, authz, store)

	userID := &commonpb.UserId{Value: []byte("test-user")}
	keyPair := model.MustGenerateKeyPair()
	authz.Add(userID, keyPair)

	tests := []struct {
		name      string
		req       *pushpb.AddTokenRequest
		setup     func() *pushpb.AddTokenRequest
		wantError codes.Code
	}{
		{
			name: "success",
			setup: func() *pushpb.AddTokenRequest {
				req := &pushpb.AddTokenRequest{
					AppInstall: &commonpb.AppInstallId{Value: "test-install"},
					TokenType:  pushpb.TokenType_FCM_ANDROID,
					PushToken:  "test-token",
				}
				require.NoError(t, keyPair.Auth(req, &req.Auth))
				return req
			},
			wantError: codes.OK,
		},
		{
			name: "unauthorized",
			setup: func() *pushpb.AddTokenRequest {
				req := &pushpb.AddTokenRequest{
					AppInstall: &commonpb.AppInstallId{Value: "test-install"},
					TokenType:  pushpb.TokenType_FCM_ANDROID,
					PushToken:  "test-token",
				}
				require.NoError(t, model.MustGenerateKeyPair().Auth(req, &req.Auth))
				return req
			},
			wantError: codes.PermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setup()

			_, err := server.AddToken(ctx, req)
			if tt.wantError != codes.OK {
				require.Error(t, err)
				require.Equal(t, tt.wantError, status.Code(err))
				return
			}

			require.NoError(t, err)

			// Verify token was stored
			tokens, err := store.GetTokens(ctx, userID)
			require.NoError(t, err)
			require.Len(t, tokens, 1)
			require.Equal(t, req.TokenType, tokens[0].Type)
			require.Equal(t, req.PushToken, tokens[0].Token)
			require.Equal(t, req.AppInstall.Value, tokens[0].AppInstallID)
		})
	}
}

func TestServer_DeleteToken(t *testing.T) {
	ctx := context.Background()
	log := zaptest.NewLogger(t)
	store := NewMemory()
	authz := auth.NewStaticAuthorizer()
	server := NewServer(log, authz, store)

	userID := &commonpb.UserId{Value: []byte("test-user")}
	keyPair := model.MustGenerateKeyPair()
	authz.Add(userID, keyPair)

	// Add a token that we can delete
	appInstall := &commonpb.AppInstallId{Value: "test-install"}
	require.NoError(t, store.AddToken(ctx, userID, appInstall, pushpb.TokenType_FCM_ANDROID, "test-token"))

	tests := []struct {
		name      string
		setup     func() *pushpb.DeleteTokenRequest
		wantError codes.Code
	}{
		{
			name: "token not found",
			setup: func() *pushpb.DeleteTokenRequest {
				req := &pushpb.DeleteTokenRequest{
					TokenType: pushpb.TokenType_FCM_ANDROID,
					PushToken: "non-existent-token",
				}
				require.NoError(t, keyPair.Auth(req, &req.Auth))
				return req
			},
			wantError: codes.OK,
		},
		{
			name: "unauthorized",
			setup: func() *pushpb.DeleteTokenRequest {
				req := &pushpb.DeleteTokenRequest{
					TokenType: pushpb.TokenType_FCM_ANDROID,
					PushToken: "test-token",
				}
				require.NoError(t, model.MustGenerateKeyPair().Auth(req, &req.Auth))
				return req
			},
			wantError: codes.PermissionDenied,
		},
		{
			name: "success",
			setup: func() *pushpb.DeleteTokenRequest {
				req := &pushpb.DeleteTokenRequest{
					TokenType: pushpb.TokenType_FCM_ANDROID,
					PushToken: "test-token",
				}
				require.NoError(t, keyPair.Auth(req, &req.Auth))
				return req
			},
			wantError: codes.OK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setup()

			_, err := server.DeleteToken(ctx, req)
			if tt.wantError != codes.OK {
				require.Error(t, err)
				require.Equal(t, tt.wantError, status.Code(err))
				return
			}

			require.NoError(t, err)

			// Verify token state
			tokens, err := store.GetTokens(ctx, userID)
			require.NoError(t, err)
			if req.PushToken == "test-token" {
				require.Empty(t, tokens)
			} else {
				require.Len(t, tokens, 1)
			}
		})
	}
}
