package memory

import (
	"context"
	"encoding/base64"
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

	baseProfile, ok := m.profiles[userIDCacheKey(id)]
	if !ok {
		return nil, profile.ErrNotFound
	}

	clonedBaseProfile := proto.Clone(baseProfile).(*profilepb.UserProfile)

	xProfile, ok := m.xProfilesByUser[userIDCacheKey(id)]
	if ok {
		clonedXProfile := proto.Clone(xProfile).(*profilepb.XProfile)
		clonedBaseProfile.SocialProfiles = append(clonedBaseProfile.SocialProfiles, &profilepb.SocialProfile{
			Type: &profilepb.SocialProfile_X{
				X: clonedXProfile,
			},
		})
	}

	return clonedBaseProfile, nil
}

func (m *Memory) SetDisplayName(_ context.Context, id *commonpb.UserId, displayName string) error {
	m.Lock()
	defer m.Unlock()

	profile, ok := m.profiles[userIDCacheKey(id)]
	if !ok {
		profile = &profilepb.UserProfile{}
	}

	// TODO: Validate eventually
	profile.DisplayName = displayName

	m.profiles[userIDCacheKey(id)] = profile

	return nil
}

func (m *Memory) LinkXAccount(ctx context.Context, userID *commonpb.UserId, xProfile *profilepb.XProfile, accessToken string) error {
	m.Lock()
	defer m.Unlock()

	existingByUser, ok := m.xProfilesByUser[userIDCacheKey(userID)]
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
	m.xProfilesByUser[userIDCacheKey(userID)] = cloned
	m.xProfilesByID[userIDCacheKey(userID)] = cloned

	return nil
}

func (m *Memory) GetXProfile(ctx context.Context, userID *commonpb.UserId) (*profilepb.XProfile, error) {
	m.Lock()
	defer m.Unlock()

	val, ok := m.xProfilesByUser[userIDCacheKey(userID)]
	if !ok {
		return nil, profile.ErrNotFound
	}

	return proto.Clone(val).(*profilepb.XProfile), nil
}

func (m *Memory) GetUserLinkedToXAccount(ctx context.Context, xUserID string) (*commonpb.UserId, error) {
	m.Lock()
	defer m.Unlock()

	for encodedUserID, xProfile := range m.xProfilesByUser {
		if xProfile.Id == xUserID {
			decodedUserID, err := base64.StdEncoding.DecodeString(encodedUserID)
			if err != nil {
				return nil, err
			}
			return &commonpb.UserId{Value: decodedUserID}, nil
		}
	}

	return nil, profile.ErrNotFound
}

func userIDCacheKey(id *commonpb.UserId) string {
	return base64.StdEncoding.EncodeToString(id.Value)
}
