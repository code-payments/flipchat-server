package chat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/query"
)

var (
	ErrChatNotFound   = errors.New("chat not found")
	ErrChatExists     = errors.New("chat exists")
	ErrMemberNotFound = errors.New("member not found")
	ErrMemberExists   = errors.New("member exists")
)

type Member struct {
	UserID        *commonpb.UserId
	AddedBy       *commonpb.UserId
	IsPushEnabled bool
	IsMuted       bool

	HasModPermission  bool
	HasSendPermission bool
}

func (m *Member) Validate() error {
	if m.UserID == nil {
		return fmt.Errorf("missing user id")
	}

	return nil
}

func (m *Member) Clone() *Member {
	return &Member{
		UserID:        proto.Clone(m.UserID).(*commonpb.UserId),
		AddedBy:       proto.Clone(m.AddedBy).(*commonpb.UserId),
		IsPushEnabled: m.IsPushEnabled,
		IsMuted:       m.IsMuted,

		HasModPermission:  m.HasModPermission,
		HasSendPermission: m.HasSendPermission,
	}
}

func (m *Member) ToProto(self *commonpb.UserId) *chatpb.Member {
	member := &chatpb.Member{
		UserId:  m.UserID,
		IsMuted: m.IsMuted,

		HasModeratorPermission: m.HasModPermission,
		HasSendPermission:      m.HasSendPermission,
	}

	if self != nil {
		member.IsSelf = bytes.Equal(member.UserId.GetValue(), self.GetValue())
	}

	return member
}

// todo: APIs for opening/closing a room
type Store interface {
	GetChatID(ctx context.Context, roomID uint64) (*commonpb.ChatId, error)

	GetChatMetadata(ctx context.Context, chatID *commonpb.ChatId) (*chatpb.Metadata, error)
	GetChatMetadataBatched(ctx context.Context, chatIDs ...*commonpb.ChatId) ([]*chatpb.Metadata, error) // todo: add paging?
	GetChatsForUser(ctx context.Context, userID *commonpb.UserId, opts ...query.Option) ([]*commonpb.ChatId, error)
	GetMembers(ctx context.Context, chatID *commonpb.ChatId) ([]*Member, error)
	GetMember(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (*Member, error)
	IsMember(_ context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error)

	CreateChat(ctx context.Context, md *chatpb.Metadata) (*chatpb.Metadata, error)
	AddMember(ctx context.Context, chatID *commonpb.ChatId, member Member) error
	RemoveMember(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) error

	SetMuteState(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId, isMuted bool) error
	IsUserMuted(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) (bool, error)

	SetSendPermission(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId, hasSendPermission bool) error
	HasSendPermission(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) (bool, error)

	SetPushState(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId, isPushEnabled bool) error
	IsPushEnabled(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) (bool, error)

	SetDisplayName(ctx context.Context, chatID *commonpb.ChatId, displayName string) error

	SetCoverCharge(ctx context.Context, chatID *commonpb.ChatId, coverCharge *commonpb.PaymentAmount) error

	AdvanceLastChatActivity(ctx context.Context, chatID *commonpb.ChatId, ts time.Time) error
}
