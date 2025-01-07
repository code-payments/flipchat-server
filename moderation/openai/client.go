package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
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

	req, err := http.NewRequest("POST", c.BaseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("non-200 status code: %d, response: %s", resp.StatusCode, string(bodyBytes))
	}

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// fmt.Println(string(responseBody))

	// Parse OpenAI-specific response
	var openaiResponse struct {
		Results []struct {
			Flagged bool `json:"flagged"`
		} `json:"results"`
	}
	if err := json.Unmarshal(responseBody, &openaiResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Map to ModerationResult
	if len(openaiResponse.Results) == 0 {
		return nil, fmt.Errorf("no results in response")
	}
	result := &moderation.ModerationResult{
		Flagged: openaiResponse.Results[0].Flagged,
	}

	return result, nil
}
