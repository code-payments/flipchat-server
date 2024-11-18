package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/profile"
)

func RunStoreTests(t *testing.T, s profile.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s profile.Store){
		testStore,
	} {
		tf(t, s)
		teardown()
	}
}

func testStore(t *testing.T, s profile.Store) {
	ctx := context.Background()

	userID := model.MustGenerateUserID()

	_, err := s.GetProfile(ctx, userID)
	require.ErrorIs(t, err, profile.ErrNotFound)

	require.NoError(t, s.SetDisplayName(ctx, userID, "my name"))

	profile, err := s.GetProfile(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, "my name", profile.DisplayName)

	require.NoError(t, s.SetDisplayName(ctx, userID, "my other name"))

	profile, err = s.GetProfile(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, "my other name", profile.DisplayName)
}
