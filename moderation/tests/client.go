package tests

import (
	"testing"

	"github.com/code-payments/flipchat-server/moderation"
)

func RunFlaggedModerationTests(t *testing.T, client moderation.ModerationClient, teardown func()) {
	for _, tf := range []func(t *testing.T, client moderation.ModerationClient){
		testFlaggedTextClassification,
		testFlaggedImageClassification,
	} {
		tf(t, client)
		teardown()
	}
}

func RunUnflaggedModerationTests(t *testing.T, client moderation.ModerationClient, teardown func()) {
	for _, tf := range []func(t *testing.T, client moderation.ModerationClient){
		testUnflaggedTextClassification,
		testUnflaggedImageClassification,
	} {
		tf(t, client)
		teardown()
	}
}

func testFlaggedTextClassification(t *testing.T, client moderation.ModerationClient) {
	t.Run("Flagged text", func(t *testing.T) {
		result, err := client.ClassifyText("This is a violent text.")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Flagged {
			t.Errorf("expected text to be flagged, got %v", result.Flagged)
		}
	})
}

func testUnflaggedTextClassification(t *testing.T, client moderation.ModerationClient) {
	t.Run("Non-flagged text", func(t *testing.T) {
		result, err := client.ClassifyText("This is a friendly text.")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Flagged {
			t.Errorf("expected text not to be flagged, got %v", result.Flagged)
		}
	})
}

func testFlaggedImageClassification(t *testing.T, client moderation.ModerationClient) {
	t.Run("Flagged image", func(t *testing.T) {
		// Using the "Cain slaying Abel" painting by Peter Paul Rubens
		// as a test image for violence
		// https://en.wikipedia.org/wiki/Violence

		result, err := client.ClassifyImage("https://upload.wikimedia.org/wikipedia/commons/5/51/Peter_Paul_Rubens_-_Cain_slaying_Abel_%28Courtauld_Institute%29.jpg")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Flagged {
			t.Errorf("expected image to be flagged, got %v", result.Flagged)
		}
	})
}

func testUnflaggedImageClassification(t *testing.T, client moderation.ModerationClient) {
	t.Run("Non-flagged image", func(t *testing.T) {
		// Using the "standard" test image "Lenna"
		// https://en.wikipedia.org/wiki/Lenna

		result, err := client.ClassifyImage("https://upload.wikimedia.org/wikipedia/en/7/7d/Lenna_%28test_image%29.png")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Flagged {
			t.Errorf("expected image not to be flagged, got %v", result.Flagged)
		}
	})
}
