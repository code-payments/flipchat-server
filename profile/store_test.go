package profile

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/code-payments/flipchat-server/account"
)

func TestStore(t *testing.T) {
	s := NewInMemory()

	userID := account.MustGenerateUserID()

	_, err := s.GetProfile(context.Background(), userID)
	require.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, s.SetDisplayName(context.Background(), userID, "my name"))

	profile, err := s.GetProfile(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, "my name", profile.DisplayName)

	require.NoError(t, s.SetDisplayName(context.Background(), userID, "my other name"))

	profile, err = s.GetProfile(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, "my other name", profile.DisplayName)
}
