package event

import (
	"sync"
	"time"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
)

type ChatEvent struct {
	ChatID    *commonpb.ChatId
	Timestamp time.Time

	ChatUpdate      *chatpb.Metadata
	PointerUpdate   *chatpb.StreamChatEventsResponse_ChatUpdate_PointerUpdate
	MemberUpdate    *chatpb.StreamChatEventsResponse_MemberUpdate
	MessageUpdate   *messagingpb.Message
	FlushedMessages []*messagingpb.Message
	IsTyping        *messagingpb.IsTyping
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
		h.OnEvent(key, e)
	}

	return nil
}
