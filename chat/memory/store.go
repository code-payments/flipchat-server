package memory

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/query"
)

type InMemoryStore struct {
	mu       sync.RWMutex
	chats    map[string]*chatpb.Metadata
	members  map[string][]*chat.Member
	nextRoom uint64

	// Indexes
	ids           map[uint64]string
	chatsByMember map[string][]string
}

func NewInMemory() chat.Store {
	return &InMemoryStore{
		chats:         map[string]*chatpb.Metadata{},
		members:       map[string][]*chat.Member{},
		nextRoom:      1,
		ids:           map[uint64]string{},
		chatsByMember: map[string][]string{},
	}
}

func (s *InMemoryStore) GetChatID(_ context.Context, roomID uint64) (*commonpb.ChatId, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	idString, ok := s.ids[roomID]
	if !ok {
		return nil, chat.ErrChatNotFound
	}

	return &commonpb.ChatId{Value: []byte(idString)}, nil
}

func (s *InMemoryStore) GetChatMetadata(_ context.Context, chatID *commonpb.ChatId) (*chatpb.Metadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	md, ok := s.chats[string(chatID.Value)]
	if !ok {
		return nil, chat.ErrChatNotFound
	}

	return proto.Clone(md).(*chatpb.Metadata), nil
}

func (s *InMemoryStore) GetChatMetadataBatched(ctx context.Context, chatIDs ...*commonpb.ChatId) ([]*chatpb.Metadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var metadata []*chatpb.Metadata
	for _, chatID := range chatIDs {
		md, ok := s.chats[string(chatID.Value)]
		if !ok {
			return nil, chat.ErrChatNotFound
		}

		metadata = append(metadata, md)
	}

	return protoutil.SliceClone(metadata), nil
}

func (s *InMemoryStore) GetChatsForUser(_ context.Context, userID *commonpb.UserId, opts ...query.Option) ([]*commonpb.ChatId, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chatIDStrings := s.chatsByMember[string(userID.Value)]

	var chatIDs []*commonpb.ChatId
	for _, chatIDString := range chatIDStrings {
		chatIDs = append(chatIDs, &commonpb.ChatId{Value: []byte(chatIDString)})
	}

	queryOpts := query.DefaultOptions()
	for _, o := range opts {
		o(&queryOpts)
	}

	slices.SortFunc(chatIDs, func(a, b *commonpb.ChatId) int {
		if queryOpts.Order == commonpb.QueryOptions_ASC {
			return bytes.Compare(a.GetValue(), b.GetValue())
		} else {
			return -1 * bytes.Compare(a.GetValue(), b.GetValue())
		}
	})

	if queryOpts.Token != nil {
		for i := range chatIDs {
			cmp := bytes.Compare(chatIDs[i].GetValue(), queryOpts.Token.GetValue())
			if queryOpts.Order == commonpb.QueryOptions_DESC {
				cmp *= -1
			}
			if cmp <= 0 {
				continue
			} else {
				chatIDs = chatIDs[i:]
				break
			}
		}
	}

	if queryOpts.Limit > 0 {
		chatIDs = chatIDs[:min(queryOpts.Limit, len(chatIDs))]
	}

	return chatIDs, nil
}

func (s *InMemoryStore) GetMembers(_ context.Context, chatID *commonpb.ChatId) ([]*chat.Member, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	members := s.members[string(chatID.Value)]

	result := make([]*chat.Member, 0, len(members))
	for _, member := range members {
		if member.IsSoftDeleted {
			continue
		}

		result = append(result, member.Clone())
	}

	return result, nil
}

func (s *InMemoryStore) GetMember(_ context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (*chat.Member, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	members := s.members[string(chatID.Value)]
	for _, member := range members {
		if bytes.Equal(member.UserID.Value, userID.Value) && !member.IsSoftDeleted {
			return member.Clone(), nil
		}
	}

	return nil, chat.ErrMemberNotFound
}

func (s *InMemoryStore) IsMember(_ context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	members := s.members[string(chatID.Value)]
	for _, member := range members {
		if bytes.Equal(member.UserID.Value, userID.Value) && !member.IsSoftDeleted {
			return true, nil
		}
	}

	return false, nil
}

func (s *InMemoryStore) CreateChat(_ context.Context, md *chatpb.Metadata) (*chatpb.Metadata, error) {
	if md.ChatId == nil {
		return nil, fmt.Errorf("must provide chat id")
	}
	if md.RoomNumber != 0 {
		return nil, errors.New("cannot create chat with room number")
	}
	if len(md.DisplayName) > 0 {
		// todo: May not always be the case in the future, but true for current flows
		return nil, errors.New("cannot create chat with display name")
	}

	md.NumUnread = 0
	md.HasMoreUnread = false
	md.IsPushEnabled = false
	md.CanDisablePush = false

	if md.OpenStatus == nil {
		md.OpenStatus = &chatpb.OpenStatus{IsCurrentlyOpen: true}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, exists := s.chats[string(md.ChatId.Value)]; exists {
		return proto.Clone(existing).(*chatpb.Metadata), chat.ErrChatExists
	}

	md.RoomNumber = s.nextRoom
	s.nextRoom++

	s.chats[string(md.ChatId.Value)] = proto.Clone(md).(*chatpb.Metadata)
	s.ids[md.RoomNumber] = string(md.ChatId.Value)

	return md, nil
}

func (s *InMemoryStore) AddMember(_ context.Context, chatID *commonpb.ChatId, member chat.Member) error {
	if member.IsSoftDeleted {
		return errors.New("cannot add soft deleted member")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.chats[string(chatID.Value)]; !exists {
		return chat.ErrChatNotFound
	}

	members := s.members[string(chatID.Value)]
	for _, m := range members {
		if bytes.Equal(m.UserID.Value, member.UserID.Value) {
			m.IsSoftDeleted = false
			m.AddedBy = member.AddedBy
			m.IsPushEnabled = true
			m.IsMuted = m.IsMuted || member.IsMuted
			m.HasModPermission = member.HasModPermission
			m.HasSendPermission = member.HasSendPermission

			if !slices.Contains(s.chatsByMember[string(member.UserID.Value)], string(chatID.Value)) {
				s.chatsByMember[string(member.UserID.Value)] = append(s.chatsByMember[string(member.UserID.Value)], string(chatID.Value))
			}

			return nil
		}
	}

	// Default for IsPushEnabled is true.
	newMember := member.Clone()
	newMember.IsPushEnabled = true

	members = append(members, newMember)
	slices.SortFunc(members, func(a, b *chat.Member) int {
		return bytes.Compare(a.UserID.Value, b.UserID.Value)
	})

	s.members[string(chatID.Value)] = members
	s.chatsByMember[string(member.UserID.Value)] = append(s.chatsByMember[string(member.UserID.Value)], string(chatID.Value))

	return nil
}

func (s *InMemoryStore) RemoveMember(_ context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove from member set.
	members := s.members[string(chatID.Value)]
	for _, m := range members {
		if bytes.Equal(m.UserID.Value, member.Value) {
			m.IsSoftDeleted = true
			m.AddedBy = nil
			m.IsPushEnabled = true
			m.HasModPermission = false
			m.HasSendPermission = false
			break
		}
	}

	// Remove from user index.
	userChats := s.chatsByMember[string(member.Value)]
	for i, m := range userChats {
		if m == string(chatID.Value) {
			userChats = slices.Delete(userChats, i, i+1)
			s.chatsByMember[string(member.Value)] = userChats
			break
		}
	}

	return nil
}

func (s *InMemoryStore) SetDisplayName(ctx context.Context, chatID *commonpb.ChatId, displayName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	md, ok := s.chats[string(chatID.Value)]
	if !ok {
		return chat.ErrChatNotFound
	}

	md.DisplayName = displayName

	return nil
}

func (s *InMemoryStore) SetCoverCharge(ctx context.Context, chatID *commonpb.ChatId, coverCharge *commonpb.PaymentAmount) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	md, ok := s.chats[string(chatID.Value)]
	if !ok {
		return chat.ErrChatNotFound
	}

	md.MessagingFee = proto.Clone(coverCharge).(*commonpb.PaymentAmount)

	return nil
}

func (s *InMemoryStore) SetOpenStatus(ctx context.Context, chatID *commonpb.ChatId, isOpen bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	md, ok := s.chats[string(chatID.Value)]
	if !ok {
		return chat.ErrChatNotFound
	}

	md.OpenStatus = &chatpb.OpenStatus{IsCurrentlyOpen: isOpen}

	return nil
}

func (s *InMemoryStore) SetMuteState(_ context.Context, chatID *commonpb.ChatId, member *commonpb.UserId, isMuted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	members := s.members[string(chatID.Value)]
	for _, m := range members {
		if bytes.Equal(m.UserID.Value, member.Value) {
			if m.IsSoftDeleted {
				return chat.ErrMemberNotFound
			}

			m.IsMuted = isMuted
			return nil
		}
	}

	return chat.ErrMemberNotFound
}

func (s *InMemoryStore) IsUserMuted(_ context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	members := s.members[string(chatID.Value)]
	for _, m := range members {
		if m.IsSoftDeleted {
			return false, chat.ErrMemberNotFound
		}

		if bytes.Equal(m.UserID.Value, member.Value) {
			return m.IsMuted, nil
		}
	}

	return false, chat.ErrMemberNotFound
}

func (s *InMemoryStore) SetSendPermission(_ context.Context, chatID *commonpb.ChatId, member *commonpb.UserId, hasSendPermission bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	members := s.members[string(chatID.Value)]
	for _, m := range members {
		if bytes.Equal(m.UserID.Value, member.Value) {
			if m.IsSoftDeleted {
				return nil
			}

			m.HasSendPermission = hasSendPermission
			return nil
		}
	}

	return chat.ErrMemberNotFound
}

func (s *InMemoryStore) HasSendPermission(_ context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	members := s.members[string(chatID.Value)]
	for _, m := range members {
		if m.IsSoftDeleted {
			return false, chat.ErrMemberNotFound
		}

		if bytes.Equal(m.UserID.Value, member.Value) {
			return m.HasSendPermission, nil
		}
	}

	return false, chat.ErrMemberNotFound
}

func (s *InMemoryStore) SetPushState(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId, isPushEnabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	members := s.members[string(chatID.Value)]
	for _, m := range members {
		if bytes.Equal(m.UserID.Value, member.Value) {
			if m.IsSoftDeleted {
				return chat.ErrMemberNotFound
			}

			m.IsPushEnabled = isPushEnabled
			return nil
		}
	}

	return chat.ErrMemberNotFound
}

func (s *InMemoryStore) IsPushEnabled(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	members := s.members[string(chatID.Value)]
	for _, m := range members {
		if bytes.Equal(m.UserID.Value, member.Value) {
			if m.IsSoftDeleted {
				return false, chat.ErrMemberNotFound
			}

			return m.IsPushEnabled, nil
		}
	}

	return false, chat.ErrMemberNotFound
}

func (s *InMemoryStore) AdvanceLastChatActivity(ctx context.Context, chatID *commonpb.ChatId, ts time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	md, ok := s.chats[string(chatID.Value)]
	if !ok {
		return chat.ErrChatNotFound
	}

	if ts.Before(md.LastActivity.AsTime()) {
		return nil
	}

	md.LastActivity = timestamppb.New(ts)

	return nil
}
