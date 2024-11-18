package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/code-payments/flipchat-server/intent"
	"github.com/code-payments/flipchat-server/model"
)

func RunStoreTests(t *testing.T, s intent.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s intent.Store){
		testStore,
	} {
		tf(t, s)
		teardown()
	}
}

func testStore(t *testing.T, store intent.Store) {
	ctx := context.Background()

	intentID := model.MustGenerateIntentID()

	isFulfilled, err := store.IsFulfilled(ctx, intentID)
	require.NoError(t, err)
	require.False(t, isFulfilled)

	require.NoError(t, store.MarkFulfilled(ctx, intentID))

	isFulfilled, err = store.IsFulfilled(ctx, intentID)
	require.NoError(t, err)
	require.True(t, isFulfilled)

	require.Equal(t, intent.ErrAlreadyFulfilled, store.MarkFulfilled(ctx, intentID))

	isFulfilled, err = store.IsFulfilled(ctx, intentID)
	require.NoError(t, err)
	require.True(t, isFulfilled)

	isFulfilled, err = store.IsFulfilled(ctx, model.MustGenerateIntentID())
	require.NoError(t, err)
	require.False(t, isFulfilled)
}
