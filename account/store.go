package account

import (
	"context"
	"errors"
	"time"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

var ErrNotFound = errors.New("not found")

type Store interface {
	// Bind binds a public key to a UserId, or returns the previously bound UserId.
	Bind(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (*commonpb.UserId, error)

	// GetUserId returns the UserId associated with a public key.
	//
	/// ErrNotFound is returned if no binding exists.
	GetUserId(ctx context.Context, pubKey *commonpb.PublicKey) (*commonpb.UserId, error)

	// GetPubKeys returns the set of public keys associated with an account.
	GetPubKeys(ctx context.Context, userID *commonpb.UserId) ([]*commonpb.PublicKey, error)

	// RemoveKey removes a key from the set of user keys.
	//
	// It is idempotent, and does not return an error if the user does not exist.
	RemoveKey(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) error

	// IsAuthorized returns whether or not a pubKey is authorized to perform actions on behalf of the user.
	IsAuthorized(ctx context.Context, userID *commonpb.UserId, pubKey *commonpb.PublicKey) (bool, error)

	// IsStaff returns whether or not a userID is a staff user
	IsStaff(ctx context.Context, userID *commonpb.UserId) (bool, error)

	// IsRegistered returns whether or not a userID is a registered account
	IsRegistered(ctx context.Context, userID *commonpb.UserId) (bool, error)

	// SetRegistrationFlag sets wether a userID is a registered account
	SetRegistrationFlag(ctx context.Context, userID *commonpb.UserId, isRegistered bool) error

	// SetNextAirdropTimestamp sets the next timestamp when a user should
	SetNextAirdropTimestamp(ctx context.Context, userID *commonpb.UserId, ts time.Time) error

	// GetNextAirdropTimestamp gets the next airdrop timestamp for a user
	GetNextAirdropTimestamp(ctx context.Context, userID *commonpb.UserId) (time.Time, error)

	// ExtendAirdropEligibility extends airdrop eligibility for a user until the
	// specified time
	ExtendAirdropEligibility(ctx context.Context, userID *commonpb.UserId, until time.Time) error

	// GetAirdropEligibilityTimestamp gets the airdrop eligibility timestamp for a user
	GetAirdropEligibilityTimestamp(ctx context.Context, userID *commonpb.UserId) (time.Time, error)

	// GetCreationTimestamp gets the creation timestamp for a user
	GetCreationTimestamp(ctx context.Context, userID *commonpb.UserId) (time.Time, error)
}
