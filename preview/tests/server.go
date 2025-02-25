package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	previewpb "github.com/code-payments/flipchat-protobuf-api/generated/go/preview/v1"
	"github.com/code-payments/flipchat-server/preview"
)

func RunUnflaggedServerTests(
	t *testing.T,
	server *preview.Server,
	store preview.Store,
	teardown func(),
) {
	for _, tf := range []func(
		t *testing.T,
		server *preview.Server,
		store preview.Store,
	){
		testGetPreviewUrl_OK,
		testGetPreviewUrl_InvalidRequest,
	} {
		tf(t, server, store)
		teardown()
	}
}

func RunFlaggedServerTests(
	t *testing.T,
	server *preview.Server,
	store preview.Store,
	teardown func(),
) {
	for _, tf := range []func(
		t *testing.T,
		server *preview.Server,
		store preview.Store,
	){
		testGetPreviewUrl_Flagged,
	} {
		tf(t, server, store)
		teardown()
	}
}

func testGetPreviewUrl_OK(t *testing.T, server *preview.Server, store preview.Store) {
	url := "https://example.com"

	// Call GetPreviewUrl
	resp, err := server.GetPreviewUrl(context.Background(), &previewpb.GetPreviewUrlRequest{
		Url: url,
	})
	require.NoError(t, err)
	require.Equal(t, previewpb.GetPreviewUrlResponse_OK, resp.Result)
	require.NotNil(t, resp.PreviewUrl)
	require.Equal(t, url, resp.PreviewUrl.Url)
	require.Equal(t, commonpb.ModerationStatus_MODERATION_APPROVED, resp.PreviewUrl.ModerationStatus)
}

func testGetPreviewUrl_InvalidRequest(t *testing.T, server *preview.Server, store preview.Store) {
	resp, err := server.GetPreviewUrl(context.Background(), &previewpb.GetPreviewUrlRequest{
		Url: "", // Invalid URL
	})
	require.NoError(t, err)
	require.Equal(t, previewpb.GetPreviewUrlResponse_INVALID_REQUEST, resp.Result)
	require.Nil(t, resp.PreviewUrl)
}

func testGetPreviewUrl_Flagged(t *testing.T, server *preview.Server, store preview.Store) {
	url := "https://flagged.com"

	// Call GetPreviewUrl
	resp, err := server.GetPreviewUrl(context.Background(), &previewpb.GetPreviewUrlRequest{
		Url: url,
	})
	require.NoError(t, err)
	require.Equal(t, previewpb.GetPreviewUrlResponse_OK, resp.Result)
	require.NotNil(t, resp.PreviewUrl)
	require.Equal(t, commonpb.ModerationStatus_MODERATION_FLAGGED, resp.PreviewUrl.ModerationStatus)
}
