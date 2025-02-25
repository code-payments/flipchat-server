package tests

import (
	"context"
	"testing"
	"time"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/preview"
	"github.com/stretchr/testify/require"
)

func RunStoreTests(
	t *testing.T,
	previewStore preview.Store,
	teardown func(),
) {

	for _, tf := range []func(
		t *testing.T,
		previewStore preview.Store,
	){
		testPreviewStore_Success,
		testPreviewStore_Exits,
		testPreviewStore_GetPreviewByID_Success,
		testPreviewStore_GetPreviewByOriginalURL_Success,
		testPreviewStore_GetPreviewByID_NotFound,
		testPreviewStore_GetPreviewByOriginalURL_NotFound,
		testPreviewStore_UpdatePreview_Success,
		testPreviewStore_UpdatePreview_NotFound,
		testPreviewStore_DeletePreview_Success,
		testPreviewStore_DeletePreview_NotFound,
	} {
		tf(t, previewStore)
		teardown()
	}
}

func testPreviewStore_Success(t *testing.T, previewStore preview.Store) {
	p := createSamplePreview(t)
	err := previewStore.CreatePreview(context.Background(), p)
	require.NoError(t, err)
}

func testPreviewStore_Exits(t *testing.T, previewStore preview.Store) {
	p := createSamplePreview(t)
	err := previewStore.CreatePreview(context.Background(), p)
	require.NoError(t, err)

	err = previewStore.CreatePreview(context.Background(), p)
	require.Equal(t, preview.ErrExists, err)
}

func testPreviewStore_GetPreviewByID_Success(t *testing.T, previewStore preview.Store) {
	p := createSamplePreview(t)
	err := previewStore.CreatePreview(context.Background(), p)
	require.NoError(t, err)

	got, err := previewStore.GetPreviewByID(context.Background(), p.ID)
	require.NoError(t, err)

	checkEqualPreview(t, p, got)
}

func testPreviewStore_GetPreviewByOriginalURL_Success(t *testing.T, previewStore preview.Store) {
	p := createSamplePreview(t)
	err := previewStore.CreatePreview(context.Background(), p)
	require.NoError(t, err)

	got, err := previewStore.GetPreviewByOriginalURL(context.Background(), p.OriginalURL)
	require.NoError(t, err)

	checkEqualPreview(t, p, got)
}

func testPreviewStore_GetPreviewByID_NotFound(t *testing.T, previewStore preview.Store) {
	_, err := previewStore.GetPreviewByID(context.Background(), "non-existent-id")
	require.Equal(t, preview.ErrNotFound, err)
}

func testPreviewStore_GetPreviewByOriginalURL_NotFound(t *testing.T, previewStore preview.Store) {
	_, err := previewStore.GetPreviewByOriginalURL(context.Background(), "https://nonexistent.url")
	require.Equal(t, preview.ErrNotFound, err)
}

func testPreviewStore_UpdatePreview_Success(t *testing.T, previewStore preview.Store) {
	p := createSamplePreview(t)
	err := previewStore.CreatePreview(context.Background(), p)
	require.NoError(t, err)

	p.Title = "Updated Title"
	p.Description = "Updated Description"
	p.Moderation = commonpb.ModerationStatus_MODERATION_APPROVED

	err = previewStore.UpdatePreview(context.Background(), p)
	require.NoError(t, err)

	got, err := previewStore.GetPreviewByID(context.Background(), p.ID)
	require.NoError(t, err)
	require.Equal(t, p.Title, got.Title)
	require.Equal(t, p.Description, got.Description)
	require.Equal(t, p.Moderation, got.Moderation)
}

func testPreviewStore_UpdatePreview_NotFound(t *testing.T, previewStore preview.Store) {
	p := createSamplePreview(t)
	err := previewStore.UpdatePreview(context.Background(), p)
	require.Equal(t, preview.ErrNotFound, err)
}

func testPreviewStore_DeletePreview_Success(t *testing.T, previewStore preview.Store) {
	p := createSamplePreview(t)
	err := previewStore.CreatePreview(context.Background(), p)
	require.NoError(t, err)

	err = previewStore.DeletePreview(context.Background(), p.ID)
	require.NoError(t, err)

	_, err = previewStore.GetPreviewByID(context.Background(), p.ID)
	require.Equal(t, preview.ErrNotFound, err)
}

func testPreviewStore_DeletePreview_NotFound(t *testing.T, previewStore preview.Store) {
	err := previewStore.DeletePreview(context.Background(), "non-existent-id")
	require.Equal(t, preview.ErrNotFound, err)
}

func checkEqualPreview(t *testing.T, expected, actual *preview.Preview) {
	require.Equal(t, expected.ID, actual.ID)
	require.Equal(t, expected.OriginalURL, actual.OriginalURL)
	require.Equal(t, expected.ContentType, actual.ContentType)
	require.Equal(t, expected.Moderation, actual.Moderation)
	require.Equal(t, expected.URL, actual.URL)
	require.Equal(t, expected.Title, actual.Title)
	require.Equal(t, expected.Description, actual.Description)
	require.Equal(t, expected.ImageURL, actual.ImageURL)
	require.Equal(t, expected.ImageHash, actual.ImageHash)
	require.Equal(t, expected.ImageWidth, actual.ImageWidth)
	require.Equal(t, expected.ImageHeight, actual.ImageHeight)
	require.WithinDuration(t, expected.CreatedAt, actual.CreatedAt, time.Second)
	require.WithinDuration(t, expected.UpdatedAt, actual.UpdatedAt, time.Second)
}

func createSamplePreview(_ *testing.T) *preview.Preview {
	return &preview.Preview{
		ID:          "preview-123",
		OriginalURL: "https://example.com",
		ContentType: commonpb.ContentType_CONTENT_TYPE_TEXT,
		Moderation:  commonpb.ModerationStatus_MODERATION_APPROVED,
		URL:         "https://example.com/preview",
		Title:       "Example Title",
		Description: "Example description for the given URL.",
		ImageURL:    "https://example.com/image.png",
		ImageHash:   "LKO2?U%2Tw=w]~RBVZRi};RPxuwH",
		ImageWidth:  800,
		ImageHeight: 600,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}
