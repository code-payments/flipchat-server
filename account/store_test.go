package account

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/protoutil"
)

func TestStore(t *testing.T) {
	store := NewInMemory()
	ctx := context.Background()

	user := MustGenerateUserID()
	keyPairs := make([]*commonpb.PublicKey, 100)
	for i := range keyPairs {
		keyPairs[i] = MustGenerateKeyPair().Proto()

		_, err := store.GetUserId(ctx, keyPairs[i])
		require.ErrorIs(t, err, ErrNotFound)

		actual, err := store.Bind(ctx, user, keyPairs[i])
		require.NoError(t, err)
		require.True(t, proto.Equal(user, actual))

		actual, err = store.GetUserId(ctx, keyPairs[i])
		require.NoError(t, err)
		require.True(t, proto.Equal(user, actual))

		// Cannot rebind without revoking first
		actual, err = store.Bind(ctx, MustGenerateUserID(), keyPairs[i])
		require.NoError(t, err)
		require.True(t, proto.Equal(user, actual))
	}

	actual, err := store.GetPubKeys(ctx, user)
	require.NoError(t, err)
	require.NoError(t, protoutil.SliceEqualError(actual, keyPairs))

	for i := range keyPairs {
		authorized, err := store.IsAuthorized(ctx, user, keyPairs[i])
		require.NoError(t, err)
		require.True(t, authorized)

		require.NoError(t, store.RemoveKey(ctx, user, keyPairs[i]))

		_, err = store.GetUserId(ctx, keyPairs[i])
		require.ErrorIs(t, err, ErrNotFound)

		authorized, err = store.IsAuthorized(ctx, user, keyPairs[i])
		require.NoError(t, err)
		require.False(t, authorized)

		require.NoError(t, store.RemoveKey(ctx, user, keyPairs[i]))
	}
}
