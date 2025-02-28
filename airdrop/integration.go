package airdrop

import (
	"context"
	"errors"
	"time"

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
	InauguralTs = time.Date(2025, 3, 7, 4, 0, 0, 0, time.UTC)
)

// todo: needs tests
type FlipchatIntegration struct {
	accounts account.Store
	pusher   push.Pusher
}

func NewFlipchatAirdropIntegration(accounts account.Store, pusher push.Pusher) codeairdrop.Integration {
	return &FlipchatIntegration{
		accounts: accounts,
		pusher:   pusher,
	}
}

func (i *FlipchatIntegration) GetOwnersToAirdropNow(ctx context.Context) ([]*codecommon.Account, uint64, error) {
	if time.Now().Before(InauguralTs) {
		return nil, 0, nil
	}

	userIDs, err := i.accounts.GetUsersToAirdrop(ctx, time.Now())
	if err == account.ErrNotFound {
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

		// Initially enabled for staff users until feature is launched
		if !isStaff {
			continue
		}

		pubKeys, err := i.accounts.GetPubKeys(ctx, userID)
		if err != nil {
			return nil, 0, err
		}

		if len(pubKeys) != 1 {
			return nil, 0, errors.New("expected 1 public key for user")
		}

		owner, err := codecommon.NewAccountFromPublicKeyBytes(pubKeys[0].Value)
		if err != nil {
			return nil, 0, err
		}
		owners = append(owners, owner)
	}

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
	}

	err := i.accounts.BatchSetNextAirdropTimestamp(ctx, GetNextAirdropTime(), userIDs...)
	if err != nil {
		return err
	}

	go push.SendWeeklyAirdropPush(context.Background(), i.pusher, Amount, userIDs...)

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
