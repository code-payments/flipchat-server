package memory

import (
	"context"
	"sync"

	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	profilepb "github.com/code-payments/flipchat-protobuf-api/generated/go/profile/v1"

	"github.com/code-payments/flipchat-server/profile"
)

type Memory struct {
	sync.Mutex

	profiles map[string]*profilepb.UserProfile
}

func NewInMemory() profile.Store {
	return &Memory{
		profiles: make(map[string]*profilepb.UserProfile),
	}
}

func (m *Memory) GetProfile(_ context.Context, id *commonpb.UserId) (*profilepb.UserProfile, error) {
	m.Lock()
	defer m.Unlock()

	val, ok := m.profiles[id.String()]
	if !ok {
		return nil, profile.ErrNotFound
	}

	return proto.Clone(val).(*profilepb.UserProfile), nil
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
