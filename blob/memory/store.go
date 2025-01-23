package memory

import (
	"context"
	"sync"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/blob"
	"github.com/code-payments/flipchat-server/model"
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

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = make(map[string]*blob.Blob)
}

func (s *store) CreateBlob(ctx context.Context, b *blob.Blob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := model.BlobIDString(b.ID)
	if _, found := s.data[key]; found {
		return blob.ErrExists
	}
	s.data[key] = b.Clone()
	return nil
}

func (s *store) GetBlob(ctx context.Context, id *commonpb.BlobId) (*blob.Blob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := model.BlobIDString(id)
	bl, found := s.data[key]
	if !found {
		return nil, blob.ErrNotFound
	}
	return bl.Clone(), nil
}
