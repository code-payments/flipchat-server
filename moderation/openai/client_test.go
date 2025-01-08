package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/code-payments/flipchat-server/moderation/tests"
	"github.com/joho/godotenv"
)

func createMockServer(response any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
}

func TestMockOpenAIClient(t *testing.T) {
	// create two mock servers that simulate the OpenAI moderation API.

	flaggedServer := createMockServer(map[string]any{
		"id":    "modr-1234567890",
		"model": "omni-moderation-latest",
		"results": []map[string]any{
			{"flagged": true}, // Always flagged
		},
	})
	defer flaggedServer.Close()

	unflaggedServer := createMockServer(map[string]any{
		"id":    "modr-0987654321",
		"model": "omni-moderation-latest",
		"results": []map[string]any{
			{"flagged": false}, // Never flagged
		},
	})
	defer unflaggedServer.Close()

	// Initialize clients
	flaggedClient := NewClient("dummy-api-key", flaggedServer.URL)
	unflaggedClient := NewClient("dummy-api-key", unflaggedServer.URL)

	// Run tests
	tests.RunFlaggedModerationTests(t, flaggedClient, func() {
		// Teardown logic...
	})

	tests.RunUnflaggedModerationTests(t, unflaggedClient, func() {
		// Teardown logic...
	})
}

// Test the real thing, requires an OpenAI API key
func TestOpenAIClient(t *testing.T) {
	if err := godotenv.Load(); err != nil {
		t.Fatalf("failed to load .env file: %v", err)
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is not set, skipping integration test")
	}

	client := NewClient(apiKey)

	tests.RunFlaggedModerationTests(t, client, func() {
		// Teardown logic...
	})

	tests.RunUnflaggedModerationTests(t, client, func() {
		// Teardown logic...
	})
}
