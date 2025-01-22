package memory

import (
	"context"
	"errors"
	"sync"

	"github.com/code-payments/flipchat-server/s3"
)

// ErrNotFound is returned when a key does not exist in the store.
var ErrNotFound = errors.New("key not found")

// store is an in-memory implementation of the s3.Store interface.
type store struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// NewInMemoryStore creates and returns a new in-memory Store.
func NewInMemoryStore() s3.Store {
	return &store{
		data: make(map[string][]byte),
	}
}

// Upload stores the data under the specified key.
func (s *store) Upload(ctx context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store a copy of the data to prevent external modifications
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	s.data[key] = dataCopy
	return nil
}

// Download retrieves the data stored under the specified key.
func (s *store) Download(ctx context.Context, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, exists := s.data[key]
	if !exists {
		return nil, ErrNotFound
	}

	// Return a copy of the data to prevent external modifications
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return dataCopy, nil
}
