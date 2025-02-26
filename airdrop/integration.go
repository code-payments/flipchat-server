package airdrop

import (
	"context"
	"errors"
	"time"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codekin "github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/flipchat-server/account"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

var (
	AirdropAmount    = codekin.ToQuarks(100)
	AirdropFrequency = 7 * 24 * time.Hour // 1 week
)

// todo: promote to code-server
type CodeAirdropIntegration interface {
	GetOwnersToAirdropNow(ctx context.Context) ([]*codecommon.Account, uint64, error)

	OnSuccess(ctx context.Context, owners []*codecommon.Account) error
}

// todo: needs tests
type FlipchatAirdropIntegration struct {
	accounts account.Store
}

func NewFlipchatAirdropIntegration(accounts account.Store) CodeAirdropIntegration {
	return &FlipchatAirdropIntegration{
		accounts: accounts,
	}
}

func (a *FlipchatAirdropIntegration) GetOwnersToAirdropNow(ctx context.Context) ([]*codecommon.Account, uint64, error) {
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

	return owners, AirdropAmount, nil
}

func (a *FlipchatAirdropIntegration) OnSuccess(ctx context.Context, owners []*codecommon.Account) error {
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
			nextAirdropAt = nextAirdropAt.Add(AirdropFrequency)

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
