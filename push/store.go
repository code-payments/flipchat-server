package push

import (
	"context"
	"sync"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	pushpb "github.com/code-payments/flipchat-protobuf-api/generated/go/push/v1"
)

// Token represents a push notification token.
//
// Tokens are bound to a (user, device) pair, identified by the AppInstallID.
type Token struct {
	Type         pushpb.TokenType
	Token        string
	AppInstallID string
}

type TokenStore interface {
	// GetTokens returns all tokens for a user.
	GetTokens(ctx context.Context, userID *commonpb.UserId) ([]Token, error)

	// AddToken adds a token for a user.
	//
	// If the token already exists for the same user and device, it will be updated.
	AddToken(ctx context.Context, userID *commonpb.UserId, appInstallID *commonpb.AppInstallId, tokenType pushpb.TokenType, token string) error

	// DeleteToken deletes a token for a user.
	DeleteToken(ctx context.Context, tokenType pushpb.TokenType, token string) error

	// ClearTokens deletes all tokens for a user.
	ClearTokens(ctx context.Context, userID *commonpb.UserId) error
}

type Memory struct {
	sync.RWMutex

	// Map of userID -> map of appInstallID -> Token
	tokens map[string]map[string]Token
}

func NewMemory() *Memory {
	return &Memory{
		tokens: make(map[string]map[string]Token),
	}
}

func (m *Memory) GetTokens(_ context.Context, userID *commonpb.UserId) ([]Token, error) {
	m.RLock()
	defer m.RUnlock()

	userTokens, ok := m.tokens[string(userID.Value)]
	if !ok {
		return nil, nil
	}

	tokens := make([]Token, 0, len(userTokens))
	for _, token := range userTokens {
		tokens = append(tokens, token)
	}

	return tokens, nil
}

func (m *Memory) AddToken(_ context.Context, userID *commonpb.UserId, appInstallID *commonpb.AppInstallId, tokenType pushpb.TokenType, token string) error {
	m.Lock()
	defer m.Unlock()

	userTokens, ok := m.tokens[string(userID.Value)]
	if !ok {
		userTokens = make(map[string]Token)
		m.tokens[string(userID.Value)] = userTokens
	}

	userTokens[appInstallID.Value] = Token{
		Type:         tokenType,
		Token:        token,
		AppInstallID: appInstallID.Value,
	}

	return nil
}

func (m *Memory) DeleteToken(_ context.Context, tokenType pushpb.TokenType, token string) error {
	m.Lock()
	defer m.Unlock()

	// Need to scan all users and devices to find matching token.
	for _, userTokens := range m.tokens {
		for appInstallID, existingToken := range userTokens {
			if existingToken.Type == tokenType && existingToken.Token == token {
				delete(userTokens, appInstallID)
			}
		}
	}

	return nil
}

func (m *Memory) ClearTokens(_ context.Context, userID *commonpb.UserId) error {
	m.Lock()
	defer m.Unlock()

	delete(m.tokens, string(userID.Value))
	return nil
}
