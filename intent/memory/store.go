package memory

import (
	"context"
	"sync"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/intent"
)

type InMemoryStore struct {
	mu               sync.RWMutex
	fulfilledIntents map[string]any
}

func NewInMemory() intent.Store {
	return &InMemoryStore{
		fulfilledIntents: make(map[string]any),
	}
}

func (s *InMemoryStore) IsFulfilled(ctx context.Context, id *commonpb.IntentId) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.fulfilledIntents[string(id.Value)]
	return ok, nil
}

func (s *InMemoryStore) MarkFulfilled(ctx context.Context, id *commonpb.IntentId) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.fulfilledIntents[string(id.Value)]; ok {
		return intent.ErrAlreadyFulfilled
	}

	s.fulfilledIntents[string(id.Value)] = struct{}{}

	return nil
}
