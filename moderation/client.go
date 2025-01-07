package moderation

// Interface for all moderation backends
type ModerationClient interface {
	ClassifyText(text string) (*ModerationResult, error)
	ClassifyImage(url string) (*ModerationResult, error)
}

// Shared moderation result structure
type ModerationResult struct {
	Flagged bool `json:"flagged"`
}
