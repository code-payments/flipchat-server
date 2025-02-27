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
)

var (
	Amount    = codekin.ToQuarks(100)
	Frequency = 7 * 24 * time.Hour // 1 week
)

// todo: needs tests
type FlipchatIntegration struct {
	accounts account.Store
}

func NewFlipchatAirdropIntegration(accounts account.Store) codeairdrop.Integration {
	return &FlipchatIntegration{
		accounts: accounts,
	}
}

func (a *FlipchatIntegration) GetOwnersToAirdropNow(ctx context.Context) ([]*codecommon.Account, uint64, error) {
	userIDs, err := a.accounts.GetUsersToAirdrop(ctx, time.Now())
	if err == account.ErrNotFound {
		return nil, 0, nil
	} else if err != nil {
		return nil, 0, err
	}

	// todo: batch fetch
	var owners []*codecommon.Account
	for _, userID := range userIDs {
		isStaff, err := a.accounts.IsStaff(ctx, userID)
		if err != nil {
			return nil, 0, err
		}

		// Initially enabled for staff users until feature is launched
		if !isStaff {
			continue
		}

		pubKeys, err := a.accounts.GetPubKeys(ctx, userID)
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

func (a *FlipchatIntegration) OnSuccess(ctx context.Context, owners ...*codecommon.Account) error {
	for _, owner := range owners {
		userID, err := a.accounts.GetUserId(ctx, &commonpb.PublicKey{Value: owner.PublicKey().ToBytes()})
		if err != nil {
			return err
		}

		creationTs, err := a.accounts.GetCreationTimestamp(ctx, userID)
		if err != nil {
			return err
		}

		// todo: something more efficient?
		now := time.Now()
		nextAirdropAt := creationTs
		for {
			nextAirdropAt = nextAirdropAt.Add(Frequency)

			if nextAirdropAt.After(now) {
				break
			}
		}

		err = a.accounts.SetNextAirdropTimestamp(ctx, userID, nextAirdropAt)
		if err != nil {
			return err
		}
	}
	return nil
}
