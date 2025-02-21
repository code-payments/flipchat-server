package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	"github.com/code-payments/flipchat-server/promoted"
)

type memoryStore struct {
	sync.RWMutex
	chatsByTopic map[string][]*promoted.PromotedChat
}

// reset clears the in-memory store (used for testing).
func (m *memoryStore) reset() {
	m.Lock()
	defer m.Unlock()
	m.chatsByTopic = make(map[string][]*promoted.PromotedChat)
}

// NewInMemory creates a new in-memory store for Promoted Chats.
func NewInMemory() promoted.Store {
	return &memoryStore{
		chatsByTopic: make(map[string][]*promoted.PromotedChat),
	}
}

// GetPromotedChats retrieves promoted chats by topic.
func (m *memoryStore) GetPromotedChats(ctx context.Context, topic string) ([]*promoted.PromotedChat, error) {
	m.RLock()
	defer m.RUnlock()

	chats, exists := m.chatsByTopic[topic]
	if !exists {
		return []*promoted.PromotedChat{}, nil
	}

	// Return a deep copy to prevent external modification.
	copies := make([]*promoted.PromotedChat, len(chats))
	for i, chat := range chats {
		copies[i] = chat.Clone()
	}

	// Sort by score (highest first)
	sort.Slice(copies, func(i, j int) bool {
		return copies[i].Score > copies[j].Score
	})

	return copies, nil
}

// PromoteChat promotes a chat (or updates the score if it already exists).
func (m *memoryStore) PromoteChat(ctx context.Context, chatID *commonpb.ChatId, topic string, score int) error {
	m.Lock()
	defer m.Unlock()

	if chatID == nil {
		return promoted.ErrInvalidChatID
	}

	if score < 0 {
		return promoted.ErrInvalidScore
	}

	if topic == "" {
		return promoted.ErrInvalidTopic
	}

	// Check if the chat already exists.
	for _, chat := range m.chatsByTopic[topic] {
		if chat.ChatID == chatID {
			chat.Score = score
			chat.UpdatedAt = time.Now()
			return nil
		}
	}

	now := time.Now()
	chat := &promoted.PromotedChat{
		ChatID:    chatID,
		Score:     score,
		Topic:     topic,
		CreatedAt: now,
		UpdatedAt: now,
	}

	m.chatsByTopic[topic] = append(m.chatsByTopic[topic], chat)
	return nil
}

// DemoteChat demotes a chat (remove it from the promoted list for the topic).
func (m *memoryStore) DemoteChat(ctx context.Context, chatID *commonpb.ChatId, topic string) error {
	m.Lock()
	defer m.Unlock()

	if chatID == nil {
		return promoted.ErrInvalidChatID
	}

	if topic == "" {
		return promoted.ErrInvalidTopic
	}

	// Find the chat and remove it from the list.
	chats := m.chatsByTopic[topic]
	for i, chat := range chats {
		if chat.ChatID == chatID {
			m.chatsByTopic[topic] = append(chats[:i], chats[i+1:]...)
			return nil
		}
	}

	return promoted.ErrNotFound
}
