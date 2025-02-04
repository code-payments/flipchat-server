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

	profiles        map[string]*profilepb.UserProfile
	xProfilesByID   map[string]*profilepb.XProfile
	xProfilesByUser map[string]*profilepb.XProfile
}

func NewInMemory() profile.Store {
	return &Memory{
		profiles:        make(map[string]*profilepb.UserProfile),
		xProfilesByID:   make(map[string]*profilepb.XProfile),
		xProfilesByUser: make(map[string]*profilepb.XProfile),
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

func (m *Memory) LinkXAccount(ctx context.Context, userID *commonpb.UserId, xProfile *profilepb.XProfile, accessToken string) error {
	m.Lock()
	defer m.Unlock()

	existingByUser, ok := m.xProfilesByUser[userID.String()]
	if ok {
		if existingByUser.Id != xProfile.Id {
			return profile.ErrExistingSocialLink
		}

		existingByUser.Username = xProfile.Username
		existingByUser.Name = xProfile.Name
		existingByUser.Description = xProfile.Description
		existingByUser.ProfilePicUrl = xProfile.ProfilePicUrl
		existingByUser.VerifiedType = xProfile.VerifiedType
		existingByUser.FollowerCount = xProfile.FollowerCount
		return nil
	}

	for key, profile := range m.xProfilesByUser {
		if profile.Id == xProfile.Id {
			delete(m.xProfilesByUser, key)
		}
	}

	cloned := proto.Clone(xProfile).(*profilepb.XProfile)
	m.xProfilesByUser[userID.String()] = cloned
	m.xProfilesByID[userID.String()] = cloned

	return nil
}

func (m *Memory) GetXProfile(ctx context.Context, userID *commonpb.UserId) (*profilepb.XProfile, error) {
	m.Lock()
	defer m.Unlock()

	val, ok := m.xProfilesByUser[userID.String()]
	if !ok {
		return nil, profile.ErrNotFound
	}

	return proto.Clone(val).(*profilepb.XProfile), nil
}
