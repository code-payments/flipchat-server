package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"

	"github.com/code-payments/flipchat-server/account"
)

func RunStoreTests(t *testing.T, s account.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s account.Store){
		testStore,
	} {
		tf(t, s)
		teardown()
	}
}

func testStore(t *testing.T, s account.Store) {
	ctx := context.Background()

	user := model.MustGenerateUserID()
	keyPairs := make([]*commonpb.PublicKey, 100)
	for i := range keyPairs {
		keyPairs[i] = model.MustGenerateKeyPair().Proto()

		_, err := s.GetUserId(ctx, keyPairs[i])
		require.ErrorIs(t, err, account.ErrNotFound)

		actual, err := s.Bind(ctx, user, keyPairs[i])
		require.NoError(t, err)
		require.True(t, proto.Equal(user, actual))

		actual, err = s.GetUserId(ctx, keyPairs[i])
		require.NoError(t, err)
		require.True(t, proto.Equal(user, actual))

		// Cannot rebind without revoking first
		actual, err = s.Bind(ctx, model.MustGenerateUserID(), keyPairs[i])
		require.NoError(t, err)
		require.True(t, proto.Equal(user, actual))
	}

	actual, err := s.GetPubKeys(ctx, user)
	require.NoError(t, err)
	require.NoError(t, protoutil.SetEqualError(actual, keyPairs))

	for i := range keyPairs {
		authorized, err := s.IsAuthorized(ctx, user, keyPairs[i])
		require.NoError(t, err)
		require.True(t, authorized)

		require.NoError(t, s.RemoveKey(ctx, user, keyPairs[i]))

		_, err = s.GetUserId(ctx, keyPairs[i])
		require.ErrorIs(t, err, account.ErrNotFound)

		authorized, err = s.IsAuthorized(ctx, user, keyPairs[i])
		require.NoError(t, err)
		require.False(t, authorized)

		require.NoError(t, s.RemoveKey(ctx, user, keyPairs[i]))
	}

	t.Logf("testRoundTrip: %d key pairs", len(keyPairs))
}
