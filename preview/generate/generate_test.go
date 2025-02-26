package generate

import (
	"context"
	"testing"

	"encoding/json"
	"github.com/stretchr/testify/require"
)

func TestPreview_Generate(t *testing.T) {
	teardown := func() {}
	RunGeneratePreviewTests(t, teardown)
}

func RunGeneratePreviewTests(
	t *testing.T,
	teardown func(),
) {
	for _, tf := range []func(
		t *testing.T,
	){
		testGeneratePreview_OK,
	} {
		tf(t)
		teardown()
	}
}

func testGeneratePreview_OK(t *testing.T) {
	url := "https://github.com/code-payments/code-vm"

	got, err := FetchPreview(context.Background(), url)

	require.NoError(t, err)
	require.NotNil(t, got)

	expected := &Result{
		OriginalURL: "https://github.com/code-payments/code-vm",
		ContentType: 1,
		Moderation:  0,
		URL:         "https://github.com/code-payments/code-vm",
		Title:       "GitHub - code-payments/code-vm: Purpose built VM for reduced fees on Solana",
		Description: "Purpose built VM for reduced fees on Solana. Contribute to code-payments/code-vm development by creating an account on GitHub.",
		ImageURL:    "https://opengraph.githubassets.com/af9db146e72f977e4e7c977cfcf588bbfafdddd240e465ff889aae122b9a33a4/code-payments/code-vm",
		ImageHash:   "UHSr},M{WCxub_WBaeofivt7ofWB?woft7Rj",
		ImageWidth:  1200,
		ImageHeight: 600,
	}

	require.Equal(t, expected.OriginalURL, got.OriginalURL)
	require.Equal(t, expected.ContentType, got.ContentType)
	require.Equal(t, expected.Moderation, got.Moderation)
	require.Equal(t, expected.URL, got.URL)
	require.Equal(t, expected.Title, got.Title)
	require.Equal(t, expected.Description, got.Description)
	require.Equal(t, expected.ImageURL, got.ImageURL)
	require.Equal(t, expected.ImageHash, got.ImageHash)
	require.Equal(t, expected.ImageWidth, got.ImageWidth)
	require.Equal(t, expected.ImageHeight, got.ImageHeight)

	// Temporarily output the response for debugging
	prettyJSON, err := json.MarshalIndent(got, "", "  ")
	require.NoError(t, err)
	t.Logf("resp: %v", string(prettyJSON))
	t.Logf("err: %v", err)

	require.True(t, false)
}
