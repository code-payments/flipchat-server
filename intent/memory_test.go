package intent

import (
	"context"
	"testing"

	"github.com/code-payments/flipchat-server/model"
	"github.com/stretchr/testify/require"
)

func TestInMemoryStore_HappyPath(t *testing.T) {
	store := NewMemory()

	intentID := model.MustGenerateIntentID()

	isFulfilled, err := store.IsFulfilled(context.Background(), intentID)
	require.NoError(t, err)
	require.False(t, isFulfilled)

	require.NoError(t, store.MarkFulfilled(context.Background(), intentID))

	isFulfilled, err = store.IsFulfilled(context.Background(), intentID)
	require.NoError(t, err)
	require.True(t, isFulfilled)

	require.Equal(t, ErrAlreadyFulfilled, store.MarkFulfilled(context.Background(), intentID))

	isFulfilled, err = store.IsFulfilled(context.Background(), intentID)
	require.NoError(t, err)
	require.True(t, isFulfilled)

	isFulfilled, err = store.IsFulfilled(context.Background(), model.MustGenerateIntentID())
	require.NoError(t, err)
	require.False(t, isFulfilled)
}
