package messaging

import (
	"context"
	"encoding/base64"
	"slices"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/event"
	"github.com/code-payments/flipchat-server/protoutil"
)

const (
	streamBufferSize = 64
	streamPingDelay  = 5 * time.Second
	streamTimeout    = time.Second
)

type Server struct {
	log   *zap.Logger
	authz auth.Authorizer

	messages MessageStore
	pointers PointerStore
	eventBus *event.Bus[*commonpb.ChatId, *event.ChatEvent]

	streamsMu sync.RWMutex
	streams   map[string][]event.Stream[*event.ChatEvent]

	messagingpb.UnimplementedMessagingServer
}

func NewServer(
	log *zap.Logger,
	authz auth.Authorizer,
	messages MessageStore,
	pointers PointerStore,
	eventBus *event.Bus[*commonpb.ChatId, *event.ChatEvent],
) *Server {
	s := &Server{
		log:   log,
		authz: authz,

		messages: messages,
		pointers: pointers,
		eventBus: eventBus,

		streams: make(map[string][]event.Stream[*event.ChatEvent]),
	}

	eventBus.AddHandler(event.HandlerFunc[*commonpb.ChatId, *event.ChatEvent](s.handleChatUpdates))

	return s
}

func (s *Server) StreamMessages(stream grpc.BidiStreamingServer[messagingpb.StreamMessagesRequest, messagingpb.StreamMessagesResponse]) error {
	ctx := stream.Context()

	req, err := protoutil.BoundedReceive[messagingpb.StreamMessagesRequest, *messagingpb.StreamMessagesRequest](
		ctx,
		stream,
		250*time.Millisecond,
	)
	if err != nil {
		return err
	}

	params := req.GetParams()
	if req.GetParams() == nil {
		return status.Error(codes.InvalidArgument, "missing parameters")
	}

	userID, err := s.authz.Authorize(ctx, params, &params.Auth)
	if err != nil {
		return err
	}

	log := s.log.With(
		zap.String("chat_id", base64.StdEncoding.EncodeToString(params.ChatId.Value)),
		zap.String("user_id", account.UserIDString(userID)),
	)

	// TODO: ChatMember verifier

	chatKey := string(params.ChatId.Value)
	userKey := string(userID.Value)

	s.streamsMu.Lock()
	chatStreams, exists := s.streams[chatKey]
	if exists {
		for _, existing := range chatStreams {
			if existing.ID() == userKey {
				s.streamsMu.Unlock()

				log.Warn("Existing stream detected on this server; aborting")
				return status.Error(codes.Aborted, "stream already exists")
			}
		}
	}

	ss := event.NewProtoEventStream[*event.ChatEvent, *messagingpb.StreamMessagesResponse_MessageBatch](
		userKey,
		streamBufferSize,
		func(e *event.ChatEvent) (*messagingpb.StreamMessagesResponse_MessageBatch, bool) {
			if e.MessageUpdate == nil {
				return nil, false
			}

			return &messagingpb.StreamMessagesResponse_MessageBatch{
				Messages: []*messagingpb.Message{e.MessageUpdate},
			}, true
		},
	)

	log.Debug("Initializing stream")

	chatStreams = append(chatStreams, ss)
	s.streams[chatKey] = chatStreams
	s.streamsMu.Unlock()

	defer func() {
		s.streamsMu.Lock()

		log.Debug("Closing streamer")

		// We check to see if the current active stream is the one that we created.
		// If it is, we can just remove it since it's closed. Otherwise, we leave it
		// be, as another StreamChatEvents() call is handling it.
		currentChatStreams := s.streams[chatKey]
		for i, liveStream := range currentChatStreams {
			if liveStream == ss {
				s.streams[chatKey] = slices.Delete(currentChatStreams, i, i+1)
				break
			}
		}

		s.streamsMu.Unlock()
	}()

	sendPingCh := time.After(0)
	streamHealthCh := protoutil.MonitorStreamHealth(ctx, log, stream, func(t *messagingpb.StreamMessagesRequest) bool {
		return t.GetPong() != nil
	})

	go s.flushMessages(ctx, params.ChatId, userID, ss)

	for {
		select {
		case batch, ok := <-ss.Channel():
			if !ok {
				log.Debug("stream closed; ending stream")
				return status.Error(codes.Aborted, "stream closed")
			}

			resp := &messagingpb.StreamMessagesResponse{
				Type: &messagingpb.StreamMessagesResponse_Messages{
					Messages: batch,
				},
			}

			if err = stream.Send(resp); err != nil {
				log.Info("Failed to forward chat message", zap.Error(err))
				return err
			}
		case <-sendPingCh:
			log.Debug("sending ping to client")

			sendPingCh = time.After(streamPingDelay)

			err := stream.Send(&messagingpb.StreamMessagesResponse{
				Type: &messagingpb.StreamMessagesResponse_Ping{
					Ping: &commonpb.ServerPing{
						Timestamp: timestamppb.Now(),
						PingDelay: durationpb.New(streamPingDelay),
					},
				},
			})
			if err != nil {
				log.Debug("stream is unhealthy; aborting")
				return status.Error(codes.Aborted, "terminating unhealthy stream")
			}
		case <-streamHealthCh:
			log.Debug("stream is unhealthy; aborting")
			return status.Error(codes.Aborted, "terminating unhealthy stream")
		case <-ctx.Done():
			log.Debug("stream context cancelled; ending stream")
			return status.Error(codes.Canceled, "")
		}
	}
}

func (s *Server) GetMessages(ctx context.Context, req *messagingpb.GetMessagesRequest) (*messagingpb.GetMessagesResponse, error) {
	_, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	// TODO: Membership check

	messages, err := s.messages.GetMessages(ctx, req.ChatId)
	if err != nil {
		s.log.Error("Failed to get all messages", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get messages")
	}

	return &messagingpb.GetMessagesResponse{
		Messages: messages,
	}, nil
}

func (s *Server) SendMessage(ctx context.Context, req *messagingpb.SendMessageRequest) (*messagingpb.SendMessageResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	msg := &messagingpb.Message{
		SenderId: userID,
		Content:  req.Content,
		Ts:       timestamppb.Now(),
	}

	if err = s.messages.PutMessage(ctx, req.ChatId, msg); err != nil {
		s.log.Error("Failed to put chat message", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to put chat message")
	}

	if err = s.eventBus.OnEvent(req.ChatId, &event.ChatEvent{ChatID: req.ChatId, MessageUpdate: msg}); err != nil {
		s.log.Warn("Failed to notify event bus", zap.Error(err))
	}

	return &messagingpb.SendMessageResponse{
		Message: msg,
	}, nil
}

func (s *Server) AdvancePointer(ctx context.Context, req *messagingpb.AdvancePointerRequest) (*messagingpb.AdvancePointerResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	advanced, err := s.pointers.AdvancePointer(ctx, req.ChatId, userID, req.Pointer)
	if err != nil {
		s.log.Error("Failed to advance pointer", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to advance pointer")
	}

	if !advanced {
		return &messagingpb.AdvancePointerResponse{}, nil
	}

	pointerUpdate := &chatpb.StreamChatEventsResponse_ChatUpdate_PointerUpdate{
		Member:  userID,
		Pointer: req.Pointer,
	}

	if err = s.eventBus.OnEvent(req.ChatId, &event.ChatEvent{ChatID: req.ChatId, PointerUpdate: pointerUpdate}); err != nil {
		s.log.Warn("Failed to notify event bus", zap.Error(err))
	}

	return &messagingpb.AdvancePointerResponse{}, nil
}

func (s *Server) NotifyIsTyping(ctx context.Context, req *messagingpb.NotifyIsTypingRequest) (*messagingpb.NotifyIsTypingResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	// TODO: Auth

	isTyping := &messagingpb.IsTyping{
		UserId:   userID,
		IsTyping: req.IsTyping,
	}

	if err = s.eventBus.OnEvent(req.ChatId, &event.ChatEvent{ChatID: req.ChatId, IsTyping: isTyping}); err != nil {
		s.log.Warn("Failed to notify event bus", zap.Error(err))
	}

	return &messagingpb.NotifyIsTypingResponse{}, nil
}

func (s *Server) flushMessages(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, stream event.Stream[*event.ChatEvent]) {
	log := s.log.With(
		zap.String("user_id", account.UserIDString(userID)),
		zap.String("chat_id", base64.StdEncoding.EncodeToString(chatID.Value)),
	)

	messages, err := s.messages.GetMessages(ctx, chatID)
	if err != nil {
		log.Warn("Failed to get messages for flush", zap.Error(err))
		return
	}

	for _, msg := range messages {
		if err = stream.Notify(&event.ChatEvent{ChatID: chatID, MessageUpdate: msg}, streamTimeout); err != nil {
			log.Info("Failed to send message to stream", zap.Error(err))
			return
		}
	}
}

func (s *Server) handleChatUpdates(chatID *commonpb.ChatId, event *event.ChatEvent) {
	// Fast pass filtering to avoid excessive locking.
	//
	// The underlying handler may filter as well, however.
	if event.MessageUpdate == nil {
		return
	}

	// TODO: Avoid global locking.
	s.streamsMu.RLock()
	defer s.streamsMu.RUnlock()

	streams := s.streams[string(chatID.Value)]
	for _, stream := range streams {
		if err := stream.Notify(event, streamTimeout); err != nil {
			s.log.Warn("Failed to notify stream", zap.Error(err), zap.String("user_id", stream.ID()))
		}
	}
}
