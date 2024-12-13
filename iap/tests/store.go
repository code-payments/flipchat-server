package tests

import (
	"context"
	"testing"
	"time"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	iappb "github.com/code-payments/flipchat-protobuf-api/generated/go/iap/v1"
	"github.com/code-payments/flipchat-server/iap"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/stretchr/testify/require"
)

func RunStoreTests(t *testing.T, s iap.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s iap.Store){
		testIapStore_HappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testIapStore_HappyPath(t *testing.T, store iap.Store) {
	expected := &iap.Purchase{
		Receipt:   &iappb.Receipt{Value: "receipt"},
		Platform:  commonpb.Platform_APPLE,
		User:      model.MustGenerateUserID(),
		Product:   iap.ProductCreateAccount,
		State:     iap.StateFulfilled,
		CreatedAt: time.Now(),
	}

	_, err := store.GetPurchase(context.Background(), expected.Receipt)
	require.Equal(t, iap.ErrNotFound, err)

	require.NoError(t, store.CreatePurchase(context.Background(), expected))

	actual, err := store.GetPurchase(context.Background(), expected.Receipt)
	require.NoError(t, err)
	require.NoError(t, protoutil.ProtoEqualError(expected.Receipt, actual.Receipt))
	require.Equal(t, expected.Platform, actual.Platform)
	require.NoError(t, protoutil.ProtoEqualError(expected.User, actual.User))
	require.Equal(t, expected.Product, actual.Product)
	require.Equal(t, expected.State, actual.State)

	require.Equal(t, iap.ErrExists, store.CreatePurchase(context.Background(), expected))
}
