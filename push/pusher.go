package push

import (
	"context"

	"firebase.google.com/go/v4/messaging"
	"go.uber.org/zap"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

type Pusher interface {
	// SendPush sends a basic visible push to a user.
	SendPushes(ctx context.Context, members []*commonpb.UserId, title, body string) error
}

type NoOpPusher struct{}

func (n *NoOpPusher) SendPushes(_ context.Context, _ []*commonpb.UserId, _, _ string) error {
	return nil
}

type FCMPusher struct {
	log    *zap.Logger
	tokens TokenStore
	client FCMClient
}

// FCMClient interface to make testing easier
type FCMClient interface {
	SendEachForMulticast(ctx context.Context, message *messaging.MulticastMessage) (*messaging.BatchResponse, error)
}

func NewFCMPusher(log *zap.Logger, tokens TokenStore, client FCMClient) *FCMPusher {
	return &FCMPusher{
		log:    log,
		tokens: tokens,
		client: client,
	}
}

func (p *FCMPusher) SendPushes(ctx context.Context, users []*commonpb.UserId, title, body string) error {
	var allPushTokens []Token
	for _, user := range users {
		tokens, err := p.tokens.GetTokens(ctx, user)
		if err != nil {
			return err
		}
		allPushTokens = append(allPushTokens, tokens...)
	}
	pushTokens := allPushTokens

	if len(pushTokens) == 0 {
		return nil
	}

	tokens := make([]string, len(pushTokens))
	for i, token := range pushTokens {
		tokens[i] = token.Token
	}

	message := &messaging.MulticastMessage{
		Tokens: tokens,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
	}

	// Send the message to all tokens
	response, err := p.client.SendEachForMulticast(ctx, message)
	if err != nil {
		return err
	}
	if response.FailureCount == 0 {
		return nil
	}

	var invalidTokens []Token
	for i, resp := range response.Responses {
		if resp.Success {
			continue
		}

		if messaging.IsUnregistered(resp.Error) {
			invalidTokens = append(invalidTokens, pushTokens[i])
		} else {
			p.log.Warn("Failed to send push notification",
				zap.Error(resp.Error),
				zap.String("token", tokens[i]),
			)
		}
	}

	if len(invalidTokens) > 0 {
		// Remove invalid tokens in the background
		go func() {
			ctx := context.Background()
			for _, token := range invalidTokens {
				_ = p.tokens.DeleteToken(ctx, token.Type, token.Token)
			}
		}()
	}

	return nil
}
