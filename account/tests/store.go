package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"

	"github.com/code-payments/flipchat-server/account"
)

func RunStoreTests(t *testing.T, s account.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s account.Store){
		testStore_keyManagement,
		testStore_registrationStatus,
		testStore_airdrops,
	} {
		tf(t, s)
		teardown()
	}
}

func testStore_keyManagement(t *testing.T, s account.Store) {
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

func testStore_registrationStatus(t *testing.T, s account.Store) {
	ctx := context.Background()

	user := model.MustGenerateUserID()

	isRegistered, err := s.IsRegistered(ctx, user)
	require.Nil(t, err)
	require.False(t, isRegistered)

	require.Equal(t, account.ErrNotFound, s.SetRegistrationFlag(ctx, user, true))

	user, err = s.Bind(ctx, user, model.MustGenerateKeyPair().Proto())
	require.NoError(t, err)

	isRegistered, err = s.IsRegistered(ctx, user)
	require.Nil(t, err)
	require.False(t, isRegistered)

	require.NoError(t, s.SetRegistrationFlag(ctx, user, true))

	isRegistered, err = s.IsRegistered(ctx, user)
	require.Nil(t, err)
	require.True(t, isRegistered)

	require.NoError(t, s.SetRegistrationFlag(ctx, user, false))

	isRegistered, err = s.IsRegistered(ctx, user)
	require.Nil(t, err)
	require.False(t, isRegistered)
}

func testStore_airdrops(t *testing.T, s account.Store) {
	ctx := context.Background()

	user := model.MustGenerateUserID()

	err := s.SetNextAirdropTimestamp(ctx, user, time.Now())
	require.Equal(t, err, account.ErrNotFound)

	_, err = s.GetNextAirdropTimestamp(ctx, user)
	require.Equal(t, err, account.ErrNotFound)

	err = s.ExtendAirdropEligibility(ctx, user, time.Now())
	require.Equal(t, err, account.ErrNotFound)

	_, err = s.GetAirdropEligibilityTimestamp(ctx, user)
	require.Equal(t, err, account.ErrNotFound)

	user, err = s.Bind(ctx, user, model.MustGenerateKeyPair().Proto())
	require.NoError(t, err)

	expected1 := time.Unix(12345, 0).UTC()
	expected2 := time.Unix(67890, 0).UTC()

	require.NoError(t, s.SetNextAirdropTimestamp(ctx, user, expected1))
	require.NoError(t, s.ExtendAirdropEligibility(ctx, user, expected2))

	actual, err := s.GetNextAirdropTimestamp(ctx, user)
	require.NoError(t, err)
	require.Equal(t, expected1, actual)

	actual, err = s.GetAirdropEligibilityTimestamp(ctx, user)
	require.NoError(t, err)
	require.Equal(t, expected2, actual)
}
