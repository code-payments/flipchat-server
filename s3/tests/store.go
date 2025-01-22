package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/code-payments/flipchat-server/s3"
)

func RunStoreTests(t *testing.T, s s3.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s s3.Store){
		testUploadAndDownload,
		testDownloadNonExistentKey,
		testOverwriteUpload,
	} {
		tf(t, s)
		teardown()
	}
}

func testUploadAndDownload(t *testing.T, s s3.Store) {
	ctx := context.Background()

	key := "testKey"
	data := []byte("testData")

	// Upload data
	err := s.Upload(ctx, key, data)
	require.NoError(t, err, "Upload should not return an error")

	// Download data
	retrievedData, err := s.Download(ctx, key)
	require.NoError(t, err, "Download should not return an error")
	require.Equal(t, data, retrievedData, "Downloaded data should match uploaded data")
}

func testDownloadNonExistentKey(t *testing.T, s s3.Store) {
	ctx := context.Background()

	nonExistentKey := "nonExistentKey"

	// Attempt to download data for a non-existent key
	data, err := s.Download(ctx, nonExistentKey)
	require.Error(t, err, "Download should return an error for non-existent key")
	require.Nil(t, data, "Downloaded data should be nil for non-existent key")
}

func testOverwriteUpload(t *testing.T, s s3.Store) {
	ctx := context.Background()

	key := "overwriteKey"
	initialData := []byte("initialData")
	newData := []byte("newData")

	// Upload initial data
	err := s.Upload(ctx, key, initialData)
	require.NoError(t, err, "Initial upload should not return an error")

	// Overwrite with new data
	err = s.Upload(ctx, key, newData)
	require.NoError(t, err, "Overwrite upload should not return an error")

	// Download data and verify it's the new data
	retrievedData, err := s.Download(ctx, key)
	require.NoError(t, err, "Download after overwrite should not return an error")
	require.Equal(t, newData, retrievedData, "Downloaded data should match the new uploaded data")
}
