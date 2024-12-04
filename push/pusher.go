package push

import (
	"context"
	"encoding/base64"

	"firebase.google.com/go/v4/messaging"
	"go.uber.org/zap"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

type Pusher interface {
	SendPushes(ctx context.Context, chatID *commonpb.ChatId, members []*commonpb.UserId, title, body string, data map[string]string) error
	SendSilentPushes(ctx context.Context, chatID *commonpb.ChatId, members []*commonpb.UserId, data map[string]string) error
}

type NoOpPusher struct{}

func (n *NoOpPusher) SendPushes(_ context.Context, _ *commonpb.ChatId, _ []*commonpb.UserId, _, _ string, _ map[string]string) error {
	return nil
}

func (n *NoOpPusher) SendSilentPushes(_ context.Context, _ *commonpb.ChatId, _ []*commonpb.UserId, _ map[string]string) error {
	return nil
}

type FCMPusher struct {
	log    *zap.Logger
	tokens TokenStore
	client FCMClient
}

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
	return p.sendMessage(ctx, chatID, users, title, body, data, false)
}

func (p *FCMPusher) SendSilentPushes(ctx context.Context, chatID *commonpb.ChatId, users []*commonpb.UserId, data map[string]string) error {
	return p.sendMessage(ctx, chatID, users, "", "", data, true)
}

func (p *FCMPusher) sendMessage(ctx context.Context, chatID *commonpb.ChatId, users []*commonpb.UserId, title, body string, data map[string]string, silent bool) error {
	pushTokens, err := p.getTokenList(ctx, users)
	if err != nil {
		return err
	}

	// A single MulticastMessage may contain up to 500 registration tokens.
	if len(pushTokens) > 500 {
		p.log.Warn("Dropping push, too many tokens", zap.Int("num_tokens", len(pushTokens)))
		return nil
	}

	if len(pushTokens) == 0 {
		p.log.Debug("Dropping push, no tokens for users", zap.Int("num_users", len(users)))
		return nil
	}

	tokens := extractTokens(pushTokens)
	message := p.buildMessage(chatID, tokens, title, body, data, silent)

	response, err := p.client.SendEachForMulticast(ctx, message)
	if err != nil {
		return err
	}

	p.log.Debug("Send pushes", zap.Int("success", response.SuccessCount), zap.Int("failed", response.FailureCount))
	p.processResponse(response, pushTokens, tokens)

	return nil
}

func (p *FCMPusher) buildMessage(chatID *commonpb.ChatId, tokens []string, title, body string, data map[string]string, silent bool) *messaging.MulticastMessage {
	aps := &messaging.Aps{
		ThreadID: base64.StdEncoding.EncodeToString(chatID.Value),
	}

	if silent {
		aps.ContentAvailable = true
	} else {
		aps.Alert = &messaging.ApsAlert{
			Title: title,
			Body:  body,
		}
	}

	return &messaging.MulticastMessage{
		Tokens: tokens,
		Notification: func() *messaging.Notification {
			if silent {
				return nil
			}
			return &messaging.Notification{
				Title: title,
				Body:  body,
			}
		}(),
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: aps,
			},
		},
		Data: data,
	}
}

func (p *FCMPusher) processResponse(response *messaging.BatchResponse, pushTokens []Token, tokens []string) {
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
		go func() {
			ctx := context.Background()
			for _, token := range invalidTokens {
				_ = p.tokens.DeleteToken(ctx, token.Type, token.Token)
			}
			p.log.Debug("Removed invalid tokens", zap.Int("count", len(invalidTokens)))
		}()
	}
}

func (p *FCMPusher) getTokenList(ctx context.Context, users []*commonpb.UserId) ([]Token, error) {
	var allPushTokens []Token
	for _, user := range users {
		tokens, err := p.tokens.GetTokens(ctx, user)
		if err != nil {
			return nil, err
		}
		allPushTokens = append(allPushTokens, tokens...)
	}
	return allPushTokens, nil
}

func extractTokens(pushTokens []Token) []string {
	tokens := make([]string, len(pushTokens))
	for i, token := range pushTokens {
		tokens[i] = token.Token
	}
	return tokens
}
