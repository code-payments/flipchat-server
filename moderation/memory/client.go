package memory

import (
	"github.com/code-payments/flipchat-server/moderation"
)

type Client struct {
	flagged bool
}

// NewClient creates a new memory-based moderation client with a predetermined
// response.
func NewClient(flagged bool) *Client {
	return &Client{flagged: flagged}
}

func (c *Client) ClassifyText(text string) (*moderation.ModerationResult, error) {
	return &moderation.ModerationResult{Flagged: c.flagged}, nil
}

func (c *Client) ClassifyImage(url string) (*moderation.ModerationResult, error) {
	return &moderation.ModerationResult{Flagged: c.flagged}, nil
}
