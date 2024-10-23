package account

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"google.golang.org/protobuf/proto"
)

var ErrNotFound = errors.New("not found")

type Store interface {
	// Bind binds a public key to a UserId, or returns the previously bound UserId.
	Bind(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (*commonpb.UserId, error)

	// GetUserId returns the UserId associated with a public key.
	//
	/// ErrNotFound is returned if no binding exists.
	GetUserId(ctx context.Context, pubKey *commonpb.PublicKey) (*commonpb.UserId, error)

	// GetPubKeys returns the set of public keys associated with an account.
	GetPubKeys(ctx context.Context, userID *commonpb.UserId) ([]*commonpb.PublicKey, error)

	// RemoveKey removes a key from the set of user keys.
	//
	// It is idempotent, and does not return an error if the user does not exist.
	RemoveKey(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) error

	// IsAuthorized returns whether or not a pubKey is authorized to perform actions on behalf of the user.
	IsAuthorized(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (bool, error)
}

type memory struct {
	sync.Mutex
	users map[string][]string
	keys  map[string]string
}

func NewInMemory() Store {
	return &memory{
		users: make(map[string][]string),
		keys:  make(map[string]string),
	}
}

func (m *memory) Bind(_ context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	m.Lock()
	defer m.Unlock()

	if prev, ok := m.keys[string(pubKey.Value)]; ok {
		return &commonpb.UserId{Value: []byte(prev)}, nil
	}

	keys := m.users[string(userID.Value)]
	keys = append(keys, string(pubKey.Value))
	m.users[string(userID.Value)] = keys

	m.keys[string(pubKey.Value)] = string(userID.Value)
	return proto.Clone(userID).(*commonpb.UserId), nil
}

func (m *memory) GetUserId(_ context.Context, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	m.Lock()
	defer m.Unlock()

	userID, ok := m.keys[string(pubKey.Value)]
	if !ok {
		return nil, ErrNotFound
	}

	return &commonpb.UserId{Value: []byte(userID)}, nil
}

func (m *memory) GetPubKeys(_ context.Context, userID *commonpb.UserId) ([]*commonpb.PublicKey, error) {
	m.Lock()
	defer m.Unlock()

	var keys []*commonpb.PublicKey
	for _, key := range m.users[string(userID.Value)] {
		keys = append(keys, &commonpb.PublicKey{Value: []byte(key)})
	}

	return keys, nil
}

func (m *memory) RemoveKey(_ context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) error {
	m.Lock()
	defer m.Unlock()

	boundUserID, exists := m.keys[string(pubKey.Value)]
	if !exists || boundUserID != string(userID.Value) {
		return nil
	}

	delete(m.keys, string(pubKey.Value))
	keys := m.users[string(userID.Value)]
	keys = slices.DeleteFunc(keys, func(e string) bool {
		return e == string(pubKey.Value)
	})
	m.users[string(userID.Value)] = keys

	return nil
}

func (m *memory) IsAuthorized(_ context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (bool, error) {
	m.Lock()
	defer m.Unlock()

	keys, ok := m.users[string(userID.Value)]
	if !ok {
		return false, nil
	}

	for _, key := range keys {
		if bytes.Equal([]byte(key), pubKey.Value) {
			return true, nil
		}
	}

	return false, nil
}
