package cache

import (
	"context"
	"time"

	"github.com/ReneKroon/ttlcache"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	profilepb "github.com/code-payments/flipchat-protobuf-api/generated/go/profile/v1"

	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/profile"
)

type Cache struct {
	db    profile.Store
	cache *ttlcache.Cache
}

func NewInCache(db profile.Store, ttl time.Duration) profile.Store {
	cache := ttlcache.NewCache()
	cache.SetTTL(ttl)
	return &Cache{
		db:    db,
		cache: cache,
	}
}

func (c *Cache) GetProfile(ctx context.Context, id *commonpb.UserId) (*profilepb.UserProfile, error) {
	cacheKey := toCacheKey(id)

	cached, ok := c.cache.Get(cacheKey)

	if !ok {
		profile, err := c.db.GetProfile(ctx, id)
		if err != nil {
			return nil, err
		}

		copied := proto.Clone(profile).(*profilepb.UserProfile)
		c.cache.Set(cacheKey, copied)

		return profile, nil
	}

	copied := proto.Clone(cached.(*profilepb.UserProfile)).(*profilepb.UserProfile)
	return copied, nil
}

func (c *Cache) SetDisplayName(ctx context.Context, id *commonpb.UserId, displayName string) error {
	c.cache.Remove(toCacheKey(id))
	return c.db.SetDisplayName(ctx, id, displayName)
}

func (c *Cache) LinkXAccount(ctx context.Context, userID *commonpb.UserId, xProfile *profilepb.XProfile, accessToken string) error {
	return c.db.LinkXAccount(ctx, userID, xProfile, accessToken)
}

func (c *Cache) GetXProfile(ctx context.Context, userID *commonpb.UserId) (*profilepb.XProfile, error) {
	return c.db.GetXProfile(ctx, userID)
}

func toCacheKey(id *commonpb.UserId) string {
	return model.UserIDString(id)
}
