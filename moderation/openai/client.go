package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/code-payments/flipchat-server/moderation"
)

const (
	defaultAPIURL = "https://api.openai.com/v1/moderations"
	model         = "omni-moderation-latest"
)

type Client struct {
	APIKey  string
	BaseURL string
}

func NewClient(apiKey string, baseURL ...string) *Client {
	url := defaultAPIURL
	if len(baseURL) > 0 {
		url = baseURL[0]
	}
	return &Client{APIKey: apiKey, BaseURL: url}
}

func (c *Client) ClassifyText(text string) (*moderation.ModerationResult, error) {
	input := map[string]interface{}{
		"model": model,
		"input": []map[string]string{{"type": "text", "text": text}},
	}
	return c.sendRequest(input)
}

func (c *Client) ClassifyImage(url string) (*moderation.ModerationResult, error) {
	input := map[string]interface{}{
		"model": model,
		"input": []map[string]interface{}{{"type": "image_url", "image_url": map[string]string{"url": url}}},
	}
	return c.sendRequest(input)
}

func (c *Client) sendRequest(input map[string]interface{}) (*moderation.ModerationResult, error) {
	jsonData, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create a new HTTP POST request
	req, err := http.NewRequest("POST", c.BaseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set necessary headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))

	// Initialize HTTP client and send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for non-200 HTTP status codes
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("non-200 status code: %d, response: %s", resp.StatusCode, string(bodyBytes))
	}

	// Read the response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Define the structure to parse OpenAI's response, including category_scores
	var openaiResponse struct {
		Results []struct {
			Flagged                   bool                `json:"flagged"`
			Categories                map[string]bool     `json:"categories"`
			CategoryScores            map[string]float64  `json:"category_scores"`
			CategoryAppliedInputTypes map[string][]string `json:"category_applied_input_types"`
		} `json:"results"`
	}

	// Unmarshal the response into the struct
	if err := json.Unmarshal(responseBody, &openaiResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Ensure there are results in the response
	if len(openaiResponse.Results) == 0 {
		return nil, fmt.Errorf("no results in response")
	}

	// Extract the first result
	firstResult := openaiResponse.Results[0]

	// Initialize ModerationResult with OpenAI's flagged value and category_scores
	result := &moderation.ModerationResult{
		Flagged:        firstResult.Flagged,
		CategoryScores: firstResult.CategoryScores,
	}

	// If OpenAI did not flag the content, check if any category_score exceeds 0.8

	// TODO: Adjust the threshold for flagging content
	if !firstResult.Flagged {
		for _, score := range firstResult.CategoryScores {
			if score > 0.1 {
				result.Flagged = true
				break
			}
		}
	}

	return result, nil
}
