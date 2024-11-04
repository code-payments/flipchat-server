package chat

import (
	"context"
	"errors"
	"fmt"

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
	UserID  *commonpb.UserId
	AddedBy *commonpb.UserId
	IsMuted bool
}

func (m *Member) Validate() error {
	if m.UserID == nil {
		return fmt.Errorf("missing user id")
	}

	return nil
}

func (m *Member) Clone() *Member {
	return &Member{
		UserID:  proto.Clone(m.UserID).(*commonpb.UserId),
		AddedBy: proto.Clone(m.AddedBy).(*commonpb.UserId),
		IsMuted: m.IsMuted,
	}
}

type Store interface {
	GetChatID(ctx context.Context, roomID uint64) (*commonpb.ChatId, error)

	GetChatMetadata(ctx context.Context, chatID *commonpb.ChatId) (*chatpb.Metadata, error)
	GetChatsForUser(ctx context.Context, userID *commonpb.UserId, opts ...query.Option) ([]*commonpb.ChatId, error)
	GetMembers(ctx context.Context, chatID *commonpb.ChatId) ([]*Member, error)
	GetMember(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (*Member, error)
	IsMember(_ context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error)

	CreateChat(ctx context.Context, md *chatpb.Metadata) (*chatpb.Metadata, error)
	AddMember(ctx context.Context, chatID *commonpb.ChatId, member Member) error
	RemoveMember(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) error

	SetMuteState(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId, isMuted bool) error
	GetMuteState(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) (bool, error)
}
