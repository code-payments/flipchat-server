package memory

import (
	"context"
	"sync"

	"github.com/code-payments/flipchat-server/blob"
)

type store struct {
	mu   sync.RWMutex
	data map[string]*blob.Blob
}

func NewInMemory() blob.Store {
	return &store{
		data: make(map[string]*blob.Blob),
	}
}

func (s *store) CreateBlob(ctx context.Context, b *blob.Blob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(b.ID)
	if _, found := s.data[key]; found {
		return blob.ErrExists
	}
	s.data[key] = b.Clone()
	return nil
}

func (s *store) GetBlob(ctx context.Context, id []byte) (*blob.Blob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := string(id)
	bl, found := s.data[key]
	if !found {
		return nil, blob.ErrNotFound
	}
	return bl.Clone(), nil
}
