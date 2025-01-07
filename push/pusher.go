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

	if response == nil {
		p.log.Debug("No response from FCM")
		return nil
	}

	p.log.Debug("Send pushes", zap.Int("success", response.SuccessCount), zap.Int("failed", response.FailureCount))
	if response.FailureCount == 0 {
		return nil
	}

	p.processResponse(response, pushTokens, tokens)

	return nil
}

func (p *FCMPusher) buildMessage(chatID *commonpb.ChatId, tokens []string, title, body string, data map[string]string, silent bool) *messaging.MulticastMessage {
	encodedChatId := base64.StdEncoding.EncodeToString(chatID.Value)

	// Apple specific payload
	aps := &messaging.Aps{
		ThreadID: encodedChatId,
	}

	data["chat_id"] = encodedChatId

	if silent {
		// Setting this to true, ensures that the notification is
		// treated as a silent push, which does not display a
		// notification banner or alert but allows the app to
		// process data in the background.

		aps.ContentAvailable = true
	} else {
		// This is a regular notification (user-visible), so include
		// the title and body in the notification payload.

		// For iOS
		aps.Alert = &messaging.ApsAlert{
			Title: title,
			Body:  body,
		}

		// For Android
		data["title"] = title
		data["body"] = body
	}

	return &messaging.MulticastMessage{
		Tokens: tokens,
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
	allPushTokens, err := p.tokens.GetTokensBatch(ctx, users...)
	if err != nil {
		return nil, err
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
