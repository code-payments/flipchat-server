package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/code-payments/flipchat-server/blob"
	"github.com/code-payments/flipchat-server/model"
)

func RunStoreTests(t *testing.T, s blob.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s blob.Store){
		testCreateAndGet,
		testCreateDuplicate,
	} {
		tf(t, s)
		teardown()
	}
}

func testCreateAndGet(t *testing.T, s blob.Store) {
	ctx := context.Background()

	blobID := model.MustGenerateBlobID()
	userID := model.MustGenerateUserID()

	// Attempt to retrieve a non-existent blob
	_, err := s.GetBlob(ctx, blobID)
	require.ErrorIs(t, err, blob.ErrNotFound)

	// Create a new blob
	now := time.Now()
	testBlob := &blob.Blob{
		ID:        blobID,
		UserID:    userID,
		Type:      blob.BlobTypeImage,
		S3URL:     "s3://test-bucket/test-key",
		Size:      12345,
		Metadata:  []byte("some metadata"),
		Flagged:   false,
		CreatedAt: now,
	}
	require.NoError(t, s.CreateBlob(ctx, testBlob))

	// Retrieve and verify
	got, err := s.GetBlob(ctx, blobID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, testBlob.UserID, got.UserID)
	require.Equal(t, testBlob.Size, got.Size)
	require.Equal(t, testBlob.Type, got.Type)
	require.Equal(t, testBlob.S3URL, got.S3URL)
	require.Equal(t, testBlob.Flagged, got.Flagged)
	require.Equal(t, testBlob.Metadata, got.Metadata)
	require.WithinDuration(t, testBlob.CreatedAt, got.CreatedAt, time.Second)
}

func testCreateDuplicate(t *testing.T, s blob.Store) {
	ctx := context.Background()

	blobID := model.MustGenerateBlobID()
	userID := model.MustGenerateUserID()

	// Create an initial blob
	testBlob := &blob.Blob{
		ID:     blobID,
		UserID: userID,
		Type:   blob.BlobTypeAudio,
		S3URL:  "s3://audio-bucket/audio-key",
		Size:   999,
	}
	require.NoError(t, s.CreateBlob(ctx, testBlob))

	// Creating again with the same ID should fail
	err := s.CreateBlob(ctx, testBlob)
	require.ErrorIs(t, err, blob.ErrExists)
}
