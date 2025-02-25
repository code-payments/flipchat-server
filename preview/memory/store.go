package memory

import (
	"context"
	"sync"
	"time"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	pg "github.com/code-payments/flipchat-server/database/postgres"
	"github.com/code-payments/flipchat-server/preview"
)

type memoryStore struct {
	sync.RWMutex
	previews         map[string]*preview.Preview // key: ID
	originalURLIndex map[string]string           // key: OriginalURL, value: ID
}

// NewInMemory creates a new in-memory store for Previews.
func NewInMemory() preview.Store {
	return &memoryStore{
		previews:         make(map[string]*preview.Preview),
		originalURLIndex: make(map[string]string),
	}
}

// GetPreviewByID retrieves a preview by ID.
func (m *memoryStore) GetPreviewByID(ctx context.Context, id *commonpb.PreviewId) (*preview.Preview, error) {
	m.RLock()
	defer m.RUnlock()

	encodedId := pg.Encode(id.Value)
	p, exists := m.previews[encodedId]
	if !exists {
		return nil, preview.ErrNotFound
	}

	// Return a copy to prevent external modification.
	cloned := p.Clone()
	return cloned, nil
}

// GetPreviewByOriginalURL retrieves a preview by the original URL.
func (m *memoryStore) GetPreviewByOriginalURL(ctx context.Context, originalURL string) (*preview.Preview, error) {
	m.RLock()
	defer m.RUnlock()

	id, exists := m.originalURLIndex[originalURL]
	if !exists {
		return nil, preview.ErrNotFound
	}

	p, exists := m.previews[id]
	if !exists {
		return nil, preview.ErrNotFound
	}

	// Return a copy to prevent external modification.
	cloned := p.Clone()
	return cloned, nil
}

// CreatePreview creates a new preview.
func (m *memoryStore) CreatePreview(ctx context.Context, p *preview.Preview) error {
	m.Lock()
	defer m.Unlock()

	encodedId := pg.Encode(p.ID.Value)
	if _, exists := m.previews[encodedId]; exists {
		return preview.ErrExists
	}

	// Check if OriginalURL already exists
	if _, exists := m.originalURLIndex[p.OriginalURL]; exists {
		return preview.ErrExists
	}

	// Store the preview
	cloned := p.Clone()
	m.previews[encodedId] = cloned
	m.originalURLIndex[p.OriginalURL] = encodedId

	return nil
}

// UpdatePreview updates an existing preview.
func (m *memoryStore) UpdatePreview(ctx context.Context, p *preview.Preview) error {
	m.Lock()
	defer m.Unlock()

	encodedId := pg.Encode(p.ID.Value)
	existing, exists := m.previews[encodedId]
	if !exists {
		return preview.ErrNotFound
	}

	// Update fields
	existing.ContentType = p.ContentType
	existing.Moderation = p.Moderation
	existing.URL = p.URL
	existing.Title = p.Title
	existing.Description = p.Description
	existing.ImageURL = p.ImageURL
	existing.ImageHash = p.ImageHash
	existing.ImageWidth = p.ImageWidth
	existing.ImageHeight = p.ImageHeight
	existing.UpdatedAt = time.Now()

	return nil
}

// DeletePreview deletes a preview by ID.
func (m *memoryStore) DeletePreview(ctx context.Context, id *commonpb.PreviewId) error {
	m.Lock()
	defer m.Unlock()

	encodedId := pg.Encode(id.Value)
	p, exists := m.previews[encodedId]
	if !exists {
		return preview.ErrNotFound
	}

	delete(m.previews, encodedId)
	delete(m.originalURLIndex, p.OriginalURL)

	return nil
}

// reset clears the in-memory store (used for testing).
func (m *memoryStore) reset() {
	m.Lock()
	defer m.Unlock()

	m.previews = make(map[string]*preview.Preview)
	m.originalURLIndex = make(map[string]string)
}
