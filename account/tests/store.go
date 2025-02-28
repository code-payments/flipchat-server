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

	user1 := model.MustGenerateUserID()
	user2 := model.MustGenerateUserID()

	require.NoError(t, s.BatchSetNextAirdropTimestamp(ctx, time.Now(), user1, user2))

	_, err := s.GetNextAirdropTimestamp(ctx, user1)
	require.Equal(t, err, account.ErrNotFound)

	err = s.ExtendAirdropEligibility(ctx, user1, time.Now())
	require.Equal(t, err, account.ErrNotFound)

	_, err = s.GetAirdropEligibilityTimestamp(ctx, user1)
	require.Equal(t, err, account.ErrNotFound)

	_, err = s.GetUsersToAirdrop(ctx, time.Now())
	require.Equal(t, err, account.ErrNotFound)

	user1, err = s.Bind(ctx, user1, model.MustGenerateKeyPair().Proto())
	require.NoError(t, err)
	user2, err = s.Bind(ctx, user2, model.MustGenerateKeyPair().Proto())
	require.NoError(t, err)

	expectedTs1 := time.Unix(5, 0).UTC()
	expectedTs2 := time.Unix(10, 0).UTC()
	expectedTs3 := time.Unix(15, 0).UTC()

	for _, user := range []*commonpb.UserId{user1, user2} {
		require.NoError(t, s.BatchSetNextAirdropTimestamp(ctx, expectedTs1, user))
		require.NoError(t, s.ExtendAirdropEligibility(ctx, user, expectedTs2))

		actualTs, err := s.GetNextAirdropTimestamp(ctx, user)
		require.NoError(t, err)
		require.Equal(t, expectedTs1, actualTs)

		actualTs, err = s.GetAirdropEligibilityTimestamp(ctx, user)
		require.NoError(t, err)
		require.Equal(t, expectedTs2, actualTs)
	}

	// Both users not registered, and ts within bound
	_, err = s.GetUsersToAirdrop(ctx, time.Unix(7, 0))
	require.Equal(t, account.ErrNotFound, err)

	require.NoError(t, s.SetRegistrationFlag(ctx, user1, true))

	// First user registered, and ts within bound
	actualUsers, err := s.GetUsersToAirdrop(ctx, time.Unix(7, 0))
	require.NoError(t, err)
	require.NoError(t, protoutil.SliceEqualError([]*commonpb.UserId{user1}, actualUsers))

	require.NoError(t, s.SetRegistrationFlag(ctx, user2, true))

	// Both users registered, and ts within bound
	actualUsers, err = s.GetUsersToAirdrop(ctx, time.Unix(7, 0))
	require.NoError(t, err)
	require.NoError(t, protoutil.SliceEqualError([]*commonpb.UserId{user1, user2}, actualUsers))

	// Ts after eligibility
	_, err = s.GetUsersToAirdrop(ctx, time.Unix(15, 0))
	require.Equal(t, account.ErrNotFound, err)

	// Ts before next scheduled airdrop
	_, err = s.GetUsersToAirdrop(ctx, time.Unix(2, 0))
	require.Equal(t, account.ErrNotFound, err)

	require.NoError(t, s.BatchSetNextAirdropTimestamp(ctx, expectedTs3, user1, user2))
	for _, user := range []*commonpb.UserId{user1, user2} {
		actualTs, err := s.GetNextAirdropTimestamp(ctx, user)
		require.NoError(t, err)
		require.Equal(t, expectedTs3, actualTs)
	}
}
