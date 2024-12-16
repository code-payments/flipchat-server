package memory

import (
	"context"
	"errors"
	"sync"

	"github.com/code-payments/flipchat-server/iap"
)

type InMemoryStore struct {
	mu        sync.RWMutex
	purchases map[string]*iap.Purchase
}

func NewInMemory() iap.Store {
	return &InMemoryStore{
		purchases: map[string]*iap.Purchase{},
	}
}

func (s *InMemoryStore) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.purchases = make(map[string]*iap.Purchase)
}

func (s *InMemoryStore) CreatePurchase(ctx context.Context, purchase *iap.Purchase) error {
	if purchase.Product != iap.ProductCreateAccount {
		return errors.New("product must be create account")
	}
	if purchase.State != iap.StateFulfilled {
		return errors.New("state must be fulfilled")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.purchases[string(purchase.ReceiptID)]
	if ok {
		return iap.ErrExists
	}

	s.purchases[string(purchase.ReceiptID)] = purchase.Clone()

	return nil
}

func (s *InMemoryStore) GetPurchase(ctx context.Context, receiptID []byte) (*iap.Purchase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	purchase, ok := s.purchases[string(receiptID)]
	if !ok {
		return nil, iap.ErrNotFound
	}
	return purchase.Clone(), nil
}
