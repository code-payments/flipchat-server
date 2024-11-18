package profile

import (
	"context"
	"errors"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	profilepb "github.com/code-payments/flipchat-protobuf-api/generated/go/profile/v1"
)

var ErrNotFound = errors.New("not found")
var ErrInvalidDisplayName = errors.New("invalid display name")

type Store interface {
	// GetProfile returns the user profile for a user, or ErrNotFound.
	GetProfile(ctx context.Context, id *commonpb.UserId) (*profilepb.UserProfile, error)

	// SetDisplayName sets the display name for a user, provided they exist.
	//
	// ErrInvalidDisplayName is returned if there is an issue with the display name.
	SetDisplayName(ctx context.Context, id *commonpb.UserId, displayName string) error
}
