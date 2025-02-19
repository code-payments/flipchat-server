package moderation

import "context"

// Interface for all moderation backends
type ModerationClient interface {
	ClassifyText(ctx context.Context, text string) (*ModerationResult, error)
	ClassifyImage(ctx context.Context, url string) (*ModerationResult, error)
}

// Shared moderation result structure
type ModerationResult struct {
	Flagged        bool `json:"flagged"`
	CategoryScores map[string]float64
}
