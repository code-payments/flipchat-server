package intent

import (
	"context"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/account"
)

// todo: Move this stuff as part of code-server when we're ready.

type CustomAntispamGuard interface {
	AllowOpenAccounts(ctx context.Context, owner *codecommon.Account) (bool, string, error)

	AllowSendPayment(ctx context.Context, owner *codecommon.Account, isPublic bool) (bool, string, error)
}

func NewFlipchatAntispamGuard(accounts account.Store) CustomAntispamGuard {
	return &FlipchatAntispamGuard{
		accounts: accounts,
	}
}

type FlipchatAntispamGuard struct {
	accounts account.Store
}

func (g *FlipchatAntispamGuard) AllowOpenAccounts(ctx context.Context, owner *codecommon.Account) (bool, string, error) {
	userID, err := g.accounts.GetUserId(ctx, &commonpb.PublicKey{Value: owner.PublicKey().ToBytes()})
	if err == account.ErrNotFound {
		return false, "public key not associated with a flipchat user", nil
	} else if err != nil {
		return false, "", err
	}

	isRegistered, err := g.accounts.IsRegistered(ctx, userID)
	if err != nil {
		return false, "", err
	}

	if !isRegistered {
		return false, "flipchat user has not completed iap for account creation", nil
	}
	return true, "", nil
}

func (g *FlipchatAntispamGuard) AllowSendPayment(_ context.Context, _ *codecommon.Account, isPublic bool) (bool, string, error) {
	if !isPublic {
		return false, "flipchat payments must be public", nil
	}
	return true, "", nil
}
