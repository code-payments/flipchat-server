package cache

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/ReneKroon/ttlcache"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/account"
)

type Cache struct {
	db                  account.Store
	pubkeyToUserCache   *ttlcache.Cache
	registeredUserCache *ttlcache.Cache
	staffFlagCache      *ttlcache.Cache
}

func NewInCache(db account.Store) account.Store {
	return &Cache{
		db:                  db,
		pubkeyToUserCache:   ttlcache.NewCache(),
		registeredUserCache: ttlcache.NewCache(),
		staffFlagCache:      ttlcache.NewCache(),
	}
}

func (c *Cache) Bind(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	return c.db.Bind(ctx, userID, pubKey)
}

func (c *Cache) GetUserId(ctx context.Context, pubKey *commonpb.PublicKey) (*commonpb.UserId, error) {
	cached, ok := c.pubkeyToUserCache.Get(string(pubKey.Value))
	if !ok {
		userID, err := c.db.GetUserId(ctx, pubKey)
		if err == nil {
			cloned := proto.Clone(userID).(*commonpb.UserId)
			c.pubkeyToUserCache.Set(string(pubKey.Value), cloned)
		}
		return userID, err
	}
	cloned := proto.Clone(cached.(*commonpb.UserId)).(*commonpb.UserId)
	return cloned, nil
}

func (c *Cache) GetPubKeys(ctx context.Context, userID *commonpb.UserId) ([]*commonpb.PublicKey, error) {
	return c.db.GetPubKeys(ctx, userID)
}

func (c *Cache) RemoveKey(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) error {
	// Because we cache forever
	return errors.New("disabled for cache implementations")
}

func (c *Cache) IsAuthorized(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (bool, error) {
	linkedUserID, err := c.GetUserId(ctx, pubKey)
	if err != nil {
		return false, err
	}
	return bytes.Equal(linkedUserID.Value, userID.Value), nil
}

func (c *Cache) IsStaff(ctx context.Context, userID *commonpb.UserId) (bool, error) {
	cached, ok := c.staffFlagCache.Get(string(userID.Value))
	if !ok {
		isStaff, err := c.db.IsStaff(ctx, userID)
		if err == nil {
			c.staffFlagCache.Set(string(userID.Value), isStaff)
		}
		return isStaff, err
	}
	return cached.(bool), nil
}

func (c *Cache) IsRegistered(ctx context.Context, userID *commonpb.UserId) (bool, error) {
	cached, ok := c.registeredUserCache.Get(string(userID.Value))
	if !ok {
		isRegistered, err := c.db.IsRegistered(ctx, userID)
		if err == nil && isRegistered {
			// Only cache positive results
			c.registeredUserCache.Set(string(userID.Value), isRegistered)
		}
		return isRegistered, err
	}
	return cached.(bool), nil
}

func (c *Cache) SetRegistrationFlag(ctx context.Context, userID *commonpb.UserId, isRegistered bool) error {
	return c.db.SetRegistrationFlag(ctx, userID, isRegistered)
}

func (c *Cache) BatchSetNextAirdropTimestamp(ctx context.Context, ts time.Time, userIDs ...*commonpb.UserId) error {
	return c.db.BatchSetNextAirdropTimestamp(ctx, ts, userIDs...)
}

func (c *Cache) GetNextAirdropTimestamp(ctx context.Context, userID *commonpb.UserId) (time.Time, error) {
	return c.db.GetNextAirdropTimestamp(ctx, userID)
}

func (c *Cache) GetUsersToAirdrop(ctx context.Context, at time.Time) ([]*commonpb.UserId, error) {
	return c.db.GetUsersToAirdrop(ctx, at)
}

func (c *Cache) ExtendAirdropEligibility(ctx context.Context, userID *commonpb.UserId, until time.Time) error {
	return c.db.ExtendAirdropEligibility(ctx, userID, until)
}

func (c *Cache) GetAirdropEligibilityTimestamp(ctx context.Context, userID *commonpb.UserId) (time.Time, error) {
	return c.db.GetAirdropEligibilityTimestamp(ctx, userID)
}
