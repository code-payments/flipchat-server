package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/iap"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"
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
		ReceiptID: []byte("receipt"),
		Platform:  commonpb.Platform_APPLE,
		User:      model.MustGenerateUserID(),
		Product:   iap.ProductCreateAccount,
		State:     iap.StateFulfilled,
		CreatedAt: time.Now(),
	}

	_, err := store.GetPurchase(context.Background(), expected.ReceiptID)
	require.Equal(t, iap.ErrNotFound, err)

	require.NoError(t, store.CreatePurchase(context.Background(), expected))

	actual, err := store.GetPurchase(context.Background(), expected.ReceiptID)
	require.NoError(t, err)
	require.Equal(t, expected.ReceiptID, actual.ReceiptID)
	require.Equal(t, expected.Platform, actual.Platform)
	require.NoError(t, protoutil.ProtoEqualError(expected.User, actual.User))
	require.Equal(t, expected.Product, actual.Product)
	require.Equal(t, expected.State, actual.State)

	require.Equal(t, iap.ErrExists, store.CreatePurchase(context.Background(), expected))
}
