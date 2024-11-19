package messaging

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"sync"

	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	"github.com/code-payments/flipchat-server/query"
)

type UserPointer struct {
	UserID  *commonpb.UserId
	Pointer *messagingpb.Pointer
}

type MessageStore interface {
	GetMessages(ctx context.Context, chatID *commonpb.ChatId, options ...query.Option) ([]*messagingpb.Message, error)
	PutMessage(ctx context.Context, chatID *commonpb.ChatId, msg *messagingpb.Message) error
	CountUnread(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, lastRead *messagingpb.MessageId) (int64, error)
}

type PointerStore interface {
	AdvancePointer(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, pointer *messagingpb.Pointer) (bool, error)
	GetPointers(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) ([]*messagingpb.Pointer, error)
	GetAllPointers(ctx context.Context, chatID *commonpb.ChatId) ([]UserPointer, error)
}

type Memory struct {
	sync.RWMutex

	messages map[string][]*messagingpb.Message
	pointers map[string]map[string][]*UserPointer
}

func NewMemory() *Memory {
	return &Memory{
		messages: make(map[string][]*messagingpb.Message),
		pointers: make(map[string]map[string][]*UserPointer),
	}
}

func (m *Memory) GetMessages(ctx context.Context, chatID *commonpb.ChatId, options ...query.Option) ([]*messagingpb.Message, error) {
	m.RLock()
	defer m.RUnlock()

	messages := m.messages[string(chatID.Value)]
	if len(messages) == 0 {
		return nil, nil
	}

	cloned := make([]*messagingpb.Message, len(messages))
	for i, message := range messages {
		cloned[i] = proto.Clone(message).(*messagingpb.Message)
	}

	return messages, nil
}

func (m *Memory) PutMessage(ctx context.Context, chatID *commonpb.ChatId, msg *messagingpb.Message) error {
	if msg.MessageId != nil {
		return fmt.Errorf("cannt provide a message id")
	}

	msg.MessageId = MustGenerateMessageID()

	m.Lock()
	defer m.Unlock()

	m.messages[string(chatID.Value)] = append(m.messages[string(chatID.Value)], proto.Clone(msg).(*messagingpb.Message))
	slices.SortFunc(m.messages[string(chatID.Value)], func(a, b *messagingpb.Message) int {
		return bytes.Compare(a.MessageId.Value, b.MessageId.Value)
	})

	return nil
}

func (m *Memory) CountUnread(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, lastRead *messagingpb.MessageId) (int64, error) {
	m.RLock()
	defer m.RUnlock()

	unread := 0
	messages := m.messages[string(chatID.Value)]

	for _, message := range messages {
		if lastRead != nil && bytes.Compare(message.MessageId.Value, lastRead.Value) <= 0 {
			continue
		}
		if message.SenderId != nil && bytes.Equal(message.SenderId.Value, userID.Value) {
			continue
		}

		unread++
	}

	return int64(unread), nil
}

func (m *Memory) AdvancePointer(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, pointer *messagingpb.Pointer) (bool, error) {
	m.Lock()
	defer m.Unlock()

	chatPtrs, ok := m.pointers[string(chatID.Value)]
	if !ok {
		chatPtrs = map[string][]*UserPointer{}
		m.pointers[string(chatID.Value)] = chatPtrs
	}

	// Note: This doesn't implicitly advance other pointers, which maybe we should consider.
	userPtrs, ok := chatPtrs[string(userID.Value)]
	for _, p := range userPtrs {
		if p.Pointer.Type != pointer.Type {
			continue
		}

		if bytes.Compare(p.Pointer.Value.Value, pointer.Value.Value) < 0 {
			p.Pointer.Value = proto.Clone(pointer.Value).(*messagingpb.MessageId)
			return true, nil
		} else {
			return false, nil
		}
	}

	userPtrs = append(userPtrs, &UserPointer{
		UserID:  proto.Clone(userID).(*commonpb.UserId),
		Pointer: proto.Clone(pointer).(*messagingpb.Pointer),
	})

	chatPtrs[string(userID.Value)] = userPtrs

	return true, nil
}

func (m *Memory) GetPointers(_ context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) ([]*messagingpb.Pointer, error) {
	m.RLock()
	defer m.RUnlock()

	chatPtrs, ok := m.pointers[string(chatID.Value)]
	if !ok {
		return nil, nil
	}

	// Note: This doesn't implicitly advance other pointers, which maybe we should consider.
	userPtrs, ok := chatPtrs[string(userID.Value)]

	var result []*messagingpb.Pointer
	for _, p := range userPtrs {
		result = append(result, proto.Clone(p.Pointer).(*messagingpb.Pointer))
	}

	return result, nil
}

func (m *Memory) GetAllPointers(_ context.Context, chatID *commonpb.ChatId) ([]UserPointer, error) {
	m.RLock()
	defer m.RUnlock()

	chatPtrs := m.pointers[string(chatID.Value)]

	var result []UserPointer
	for _, userPtrs := range chatPtrs {
		for _, ptr := range userPtrs {
			result = append(result, UserPointer{
				UserID:  proto.Clone(ptr.UserID).(*commonpb.UserId),
				Pointer: proto.Clone(ptr.Pointer).(*messagingpb.Pointer),
			})
		}
	}

	return result, nil
}
