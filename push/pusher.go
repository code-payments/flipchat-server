package push

import (
	"context"
	"encoding/base64"

	"firebase.google.com/go/v4/messaging"
	"go.uber.org/zap"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

type Pusher interface {
	// SendPushes sends a basic visible push to a user.
	SendPushes(ctx context.Context, chatID *commonpb.ChatId, members []*commonpb.UserId, title, body string, data map[string]string) error
}

type NoOpPusher struct{}

func (n *NoOpPusher) SendPushes(_ context.Context, _ *commonpb.ChatId, _ []*commonpb.UserId, _, _ string) error {
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

func (p *FCMPusher) SendPushes(ctx context.Context, chatID *commonpb.ChatId, users []*commonpb.UserId, title, body string, data map[string]string) error {
	pushTokens, err := p.tokens.GetTokensBatch(ctx, users...)
	if err != nil {
		p.log.Warn("Failed to get push tokens", zap.Error(err))
		return err
	}

	if len(pushTokens) == 0 {
		p.log.Debug("Dropping push, no tokens for users", zap.Int("num_users", len(users)))
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
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Alert: &messaging.ApsAlert{
						Title: title,
						Body:  body,
					},
					ThreadID: base64.StdEncoding.EncodeToString(chatID.Value),
				},
			},
		},
		Data: data,
	}

	// Send the message to all tokens
	response, err := p.client.SendEachForMulticast(ctx, message)
	if err != nil {
		return err
	}

	p.log.Debug("Send pushes", zap.Int("success", response.SuccessCount), zap.Int("failed", response.FailureCount))
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
			p.log.Debug("Removed invalid tokens", zap.Int("count", len(invalidTokens)))
		}()
	}

	return nil
}
