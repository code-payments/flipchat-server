package event

import (
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
	"github.com/code-payments/flipchat-server/protoutil"
)

type ChatEvent struct {
	ChatID    *commonpb.ChatId
	Timestamp time.Time

	MetadataUpdates []*chatpb.MetadataUpdate
	MemberUpdates   []*chatpb.MemberUpdate
	MessageUpdate   *messagingpb.Message
	FlushedMessages []*messagingpb.Message
	PointerUpdate   *chatpb.StreamChatEventsResponse_ChatUpdate_PointerUpdate
	IsTyping        *messagingpb.IsTyping
}

func (e *ChatEvent) Clone() *ChatEvent {
	return &ChatEvent{
		ChatID:    proto.Clone(e.ChatID).(*commonpb.ChatId),
		Timestamp: e.Timestamp,

		MetadataUpdates: protoutil.SliceClone(e.MetadataUpdates),
		MemberUpdates:   protoutil.SliceClone(e.MemberUpdates),
		MessageUpdate:   proto.Clone(e.MessageUpdate).(*messagingpb.Message),
		FlushedMessages: protoutil.SliceClone(e.FlushedMessages),
		PointerUpdate:   proto.Clone(e.PointerUpdate).(*chatpb.StreamChatEventsResponse_ChatUpdate_PointerUpdate),
		IsTyping:        proto.Clone(e.IsTyping).(*messagingpb.IsTyping),
	}
}

type Handler[Key, Event any] interface {
	OnEvent(key Key, e Event)
}

// HandlerFunc is an adapter to allow the use of ordinary
// functions as Handlers.
type HandlerFunc[Key, Event any] func(Key, Event)

// OnEvent calls f(key, e).
func (f HandlerFunc[Key, Event]) OnEvent(key Key, e Event) {
	f(key, e)
}

type Bus[Key, Event any] struct {
	keyFor func(Key) []byte

	handlersMu sync.RWMutex
	handlers   []Handler[Key, Event]
}

func NewBus[Key, Event any](keyFor func(Key) []byte) *Bus[Key, Event] {
	return &Bus[Key, Event]{
		keyFor:     keyFor,
		handlersMu: sync.RWMutex{},
		handlers:   nil,
	}
}

func (b *Bus[Key, Event]) AddHandler(h Handler[Key, Event]) {
	b.handlersMu.Lock()
	b.handlers = append(b.handlers, h)
	b.handlersMu.Unlock()
}

func (b *Bus[Key, Event]) OnEvent(key Key, e Event) error {
	b.handlersMu.RLock()
	// Copy handlers to prevent race conditions
	handlers := make([]Handler[Key, Event], len(b.handlers))
	copy(handlers, b.handlers)
	b.handlersMu.RUnlock()

	// Execute handlers outside the lock
	for _, h := range handlers {
		go h.OnEvent(key, e)
	}

	return nil
}
