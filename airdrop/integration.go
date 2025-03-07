package airdrop

import (
	"context"
	"encoding/base64"
	"time"

	"go.uber.org/zap"

	codeairdrop "github.com/code-payments/code-server/pkg/code/async/airdrop"
	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codekin "github.com/code-payments/code-server/pkg/kin"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/push"
)

var (
	Amount      = codekin.ToQuarks(100)
	Frequency   = 7 * 24 * time.Hour // 1 week
	InauguralTs = time.Date(2025, 3, 7, 16, 0, 0, 0, time.UTC)
)

// todo: needs tests
type FlipchatIntegration struct {
	log      *zap.Logger
	accounts account.Store
	pusher   push.Pusher
}

func NewFlipchatAirdropIntegration(log *zap.Logger, accounts account.Store, pusher push.Pusher) codeairdrop.Integration {
	return &FlipchatIntegration{
		log:      log,
		accounts: accounts,
		pusher:   pusher,
	}
}

func (i *FlipchatIntegration) GetOwnersToAirdropNow(ctx context.Context) ([]*codecommon.Account, uint64, error) {
	userIDs, err := i.accounts.GetUsersToAirdrop(ctx, time.Now())
	if err == account.ErrNotFound {
		i.log.Debug("No users to airdrop")
		return nil, 0, nil
	} else if err != nil {
		return nil, 0, err
	}

	// todo: batch fetch
	var owners []*codecommon.Account
	for _, userID := range userIDs {
		isStaff, err := i.accounts.IsStaff(ctx, userID)
		if err != nil {
			return nil, 0, err
		}

		// Initially enabled for staff before feature is launched
		if !isStaff && time.Now().Before(InauguralTs) {
			continue
		}

		pubKeys, err := i.accounts.GetPubKeys(ctx, userID)
		if err != nil {
			return nil, 0, err
		}

		if len(pubKeys) != 1 {
			i.log.Info(
				"Skipping airdrop to user with unexpected public key count",
				zap.String("user_id", base64.StdEncoding.EncodeToString(userID.Value)),
				zap.Int("count", len(pubKeys)),
			)
			continue
		}

		owner, err := codecommon.NewAccountFromPublicKeyBytes(pubKeys[0].Value)
		if err != nil {
			return nil, 0, err
		}
		owners = append(owners, owner)

		i.log.Debug(
			"Airdropping to user",
			zap.String("user_id", base64.StdEncoding.EncodeToString(userID.Value)),
			zap.String("public_key", owner.PublicKey().ToBase58()),
		)
	}

	i.log.Debug("Airdropping to users", zap.Int("count", len(owners)))

	return owners, Amount, nil
}

func (i *FlipchatIntegration) OnSuccess(ctx context.Context, owners ...*codecommon.Account) error {
	var userIDs []*commonpb.UserId
	for _, owner := range owners {
		userID, err := i.accounts.GetUserId(ctx, &commonpb.PublicKey{Value: owner.PublicKey().ToBytes()})
		if err != nil {
			return err
		}
		userIDs = append(userIDs, userID)

		i.log.Debug(
			"Airdropped user",
			zap.String("user_id", base64.StdEncoding.EncodeToString(userID.Value)),
			zap.String("public_key", owner.PublicKey().ToBase58()),
		)
	}

	err := i.accounts.BatchSetNextAirdropTimestamp(ctx, GetNextAirdropTime(), userIDs...)
	if err != nil {
		return err
	}

	go push.SendWeeklyAirdropPush(context.Background(), i.pusher, Amount, userIDs...)

	i.log.Debug("Airdropped users", zap.Int("count", len(owners)))

	return nil
}

func GetNextAirdropTime() time.Time {
	return getNextAirdropTime(InauguralTs)
}

// todo: something more efficient?
func getNextAirdropTime(inauguralTs time.Time) time.Time {
	now := time.Now()
	ts := inauguralTs
	for {
		if now.Before(ts) {
			return ts
		}
		ts = ts.Add(Frequency)
	}
}
