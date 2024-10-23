package profile

import (
	"context"
	"errors"
	"sync"

	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	profilepb "github.com/code-payments/flipchat-protobuf-api/generated/go/profile/v1"
)

var ErrNotFound = errors.New("not found")
var ErrInvalidDisplayName = errors.New("invalid display name")

type Store interface {
	// GetProfile returns the user profile for a user, or ErrNotFound.
	GetProfile(ctx context.Context, id *commonpb.UserId) (*profilepb.UserProfile, error)

	// SetDisplayName sets the display name for a user, provided they exist.
	//
	// ErrInvalidDisplayName is returned if there is an issue with the display name.
	SetDisplayName(ctx context.Context, id *commonpb.UserId, displayName string) error
}

type Memory struct {
	sync.Mutex

	profiles map[string]*profilepb.UserProfile
}

func NewInMemory() *Memory {
	return &Memory{
		profiles: make(map[string]*profilepb.UserProfile),
	}
}

func (m *Memory) GetProfile(_ context.Context, id *commonpb.UserId) (*profilepb.UserProfile, error) {
	m.Lock()
	defer m.Unlock()

	profile, ok := m.profiles[id.String()]
	if !ok {
		return nil, ErrNotFound
	}

	return proto.Clone(profile).(*profilepb.UserProfile), nil
}

func (m *Memory) SetDisplayName(_ context.Context, id *commonpb.UserId, displayName string) error {
	m.Lock()
	defer m.Unlock()

	profile, ok := m.profiles[id.String()]
	if !ok {
		profile = &profilepb.UserProfile{}
	}

	// TODO: Validate eventually
	profile.DisplayName = displayName

	m.profiles[id.String()] = profile

	return nil
}
