package memory

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"sort"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	"github.com/code-payments/flipchat-server/messaging"
	"github.com/code-payments/flipchat-server/query"
)

type MessagesById []*messagingpb.Message

func (a MessagesById) Len() int      { return len(a) }
func (a MessagesById) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a MessagesById) Less(i, j int) bool {
	return bytes.Compare(a[i].MessageId.Value, a[j].MessageId.Value) < 0
}

type Memory struct {
	sync.RWMutex

	messages map[string][]*messagingpb.Message
	pointers map[string]map[string][]*messaging.UserPointer
}

func NewInMemory() *Memory {
	return &Memory{
		messages: make(map[string][]*messagingpb.Message),
		pointers: make(map[string]map[string][]*messaging.UserPointer),
	}
}

func (m *Memory) GetMessage(ctx context.Context, chatID *commonpb.ChatId, messageID *messagingpb.MessageId) (*messagingpb.Message, error) {
	m.RLock()
	defer m.RUnlock()

	var found *messagingpb.Message
	messages := m.messages[string(chatID.Value)]
	for _, message := range messages {
		if bytes.Equal(messageID.Value, message.MessageId.Value) {
			found = message
			break
		}
	}

	if found == nil {
		return nil, messaging.ErrMessageNotFound
	}
	return proto.Clone(found).(*messagingpb.Message), nil
}

func (m *Memory) GetMessages(ctx context.Context, chatID *commonpb.ChatId, options ...query.Option) ([]*messagingpb.Message, error) {
	appliedOptions := query.ApplyOptions(options...)

	m.RLock()
	defer m.RUnlock()

	messages := m.messages[string(chatID.Value)]
	if len(messages) == 0 {
		return nil, nil
	}

	cloned := make([]*messagingpb.Message, 0)
	for _, message := range messages {
		if appliedOptions.Token != nil && appliedOptions.Order == commonpb.QueryOptions_ASC && bytes.Compare(message.MessageId.Value, appliedOptions.Token.Value) <= 0 {
			continue
		}

		if appliedOptions.Token != nil && appliedOptions.Order == commonpb.QueryOptions_DESC && bytes.Compare(message.MessageId.Value, appliedOptions.Token.Value) >= 0 {
			continue
		}

		cloned = append(cloned, proto.Clone(message).(*messagingpb.Message))
	}

	sorted := MessagesById(cloned)
	if appliedOptions.Order == commonpb.QueryOptions_DESC {
		sort.Sort(sort.Reverse(sorted))
	}

	limited := sorted
	if len(limited) > appliedOptions.Limit {
		limited = limited[:appliedOptions.Limit]
	}
	return limited, nil
}

func (m *Memory) PutMessage(ctx context.Context, chatID *commonpb.ChatId, msg *messagingpb.Message) (*messagingpb.Message, error) {
	m.Lock()
	defer m.Unlock()

	msg = proto.Clone(msg).(*messagingpb.Message)

	if msg.MessageId != nil {
		return nil, fmt.Errorf("cannt provide a message id")
	}

	if msg.Ts == nil {
		msg.Ts = timestamppb.Now()
	}

	msg.MessageId = messaging.MustGenerateMessageID()

	m.messages[string(chatID.Value)] = append(m.messages[string(chatID.Value)], proto.Clone(msg).(*messagingpb.Message))
	slices.SortFunc(m.messages[string(chatID.Value)], func(a, b *messagingpb.Message) int {
		return bytes.Compare(a.MessageId.Value, b.MessageId.Value)
	})

	return msg, nil
}

func (m *Memory) PutMessageLegacy(ctx context.Context, chatID *commonpb.ChatId, msg *messagingpb.Message) (*messagingpb.Message, error) {
	// Memory store doesn't support legacy messages.
	return m.PutMessage(ctx, chatID, msg)
}

func (m *Memory) CountUnread(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, lastRead *messagingpb.MessageId, maxValue int64) (int64, error) {
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

	if maxValue >= 0 && int64(unread) > maxValue {
		return maxValue, nil
	}
	return int64(unread), nil
}

func (m *Memory) AdvancePointer(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, pointer *messagingpb.Pointer) (bool, error) {
	m.Lock()
	defer m.Unlock()

	chatPtrs, ok := m.pointers[string(chatID.Value)]
	if !ok {
		chatPtrs = map[string][]*messaging.UserPointer{}
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

	userPtrs = append(userPtrs, &messaging.UserPointer{
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

func (m *Memory) GetAllPointers(_ context.Context, chatID *commonpb.ChatId) ([]messaging.UserPointer, error) {
	m.RLock()
	defer m.RUnlock()

	chatPtrs := m.pointers[string(chatID.Value)]

	var result []messaging.UserPointer
	for _, userPtrs := range chatPtrs {
		for _, ptr := range userPtrs {
			result = append(result, messaging.UserPointer{
				UserID:  proto.Clone(ptr.UserID).(*commonpb.UserId),
				Pointer: proto.Clone(ptr.Pointer).(*messagingpb.Pointer),
			})
		}
	}

	return result, nil
}
