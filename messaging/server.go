package messaging

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	codedata "github.com/code-payments/code-server/pkg/code/data"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/event"
	"github.com/code-payments/flipchat-server/intent"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/query"
)

const (
	streamBufferSize = 64
	streamPingDelay  = 5 * time.Second
	streamTimeout    = time.Second

	maxMessageEventBatchSize = 1024
	flushedMessageBatchSize  = maxMessageEventBatchSize
)

type Server struct {
	log      *zap.Logger
	authz    auth.Authorizer
	rpcAuthz auth.Messaging

	accounts account.Store
	intents  intent.Store
	messages MessageStore
	pointers PointerStore
	codeData codedata.Provider

	eventBus *event.Bus[*commonpb.ChatId, *event.ChatEvent]

	streamsMu sync.RWMutex
	streams   map[string][]event.Stream[*event.ChatEvent]

	messagingpb.UnimplementedMessagingServer
}

func NewServer(
	log *zap.Logger,
	authz auth.Authorizer,
	rpcAuthz auth.Messaging,
	accounts account.Store,
	intents intent.Store,
	messages MessageStore,
	pointers PointerStore,
	codeData codedata.Provider,
	eventBus *event.Bus[*commonpb.ChatId, *event.ChatEvent],
) *Server {
	s := &Server{
		log:      log,
		authz:    authz,
		rpcAuthz: rpcAuthz,

		accounts: accounts,
		intents:  intents,
		messages: messages,
		pointers: pointers,
		codeData: codeData,

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

	minLogLevel := zap.DebugLevel
	isStaff, _ := s.accounts.IsStaff(ctx, userID)
	if isStaff {
		minLogLevel = zap.InfoLevel
	}

	streamID := uuid.New()

	log := s.log.With(
		zap.String("chat_id", base64.StdEncoding.EncodeToString(params.ChatId.Value)),
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("stream_id", streamID.String()),
	)

	allow, _, err := s.rpcAuthz.CanStreamMessages(ctx, params.ChatId, userID)
	if err != nil {
		return status.Error(codes.Internal, "failed to do rpc authz checks")
	} else if !allow {
		return stream.Send(&messagingpb.StreamMessagesResponse{
			Type: &messagingpb.StreamMessagesResponse_Error{
				Error: &messagingpb.StreamMessagesResponse_StreamError{
					Code: messagingpb.StreamMessagesResponse_StreamError_DENIED,
				},
			},
		})
	}

	chatKey := string(params.ChatId.Value)
	userKey := string(userID.Value)

	s.streamsMu.Lock()
	chatStreams, exists := s.streams[chatKey]
	if exists {
		for i, existing := range chatStreams {
			if existing.ID() == userKey {
				chatStreams = slices.Delete(chatStreams, i, i+1)

				existing.Close()
				log.Info("Closed previous stream")
				break
			}
		}
	}

	ss := event.NewProtoEventStream[*event.ChatEvent, *messagingpb.MessageBatch](
		userKey,
		streamBufferSize,
		func(e *event.ChatEvent) (*messagingpb.MessageBatch, bool) {
			var messages []*messagingpb.Message

			// Only one of these should be not nil at a time
			if e.MessageUpdate != nil {
				messages = append(messages, e.MessageUpdate)
			}
			if len(e.FlushedMessages) > 0 {
				messages = append(messages, e.FlushedMessages...)
			}

			if len(messages) == 0 {
				return nil, false
			}
			if len(messages) > maxMessageEventBatchSize {
				log.Warn("Message batch size exceeds proto limit")
				return nil, false
			}
			return &messagingpb.MessageBatch{
				Messages: messages,
			}, true
		},
	)

	log.Log(minLogLevel, "Initializing stream")

	chatStreams = append(chatStreams, ss)
	s.streams[chatKey] = chatStreams
	s.streamsMu.Unlock()

	defer func() {
		s.streamsMu.Lock()

		log.Log(minLogLevel, "Closing streamer")

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

	for {
		select {
		case batch, ok := <-ss.Channel():
			if !ok {
				log.Log(minLogLevel, "Stream closed; ending stream")
				return status.Error(codes.Aborted, "stream closed")
			}

			resp := &messagingpb.StreamMessagesResponse{
				Type: &messagingpb.StreamMessagesResponse_Messages{
					Messages: batch,
				},
			}

			log.Log(minLogLevel, "Forwarding chat messages", zap.Int("batch_size", len(batch.Messages)))
			if err = stream.Send(resp); err != nil {
				log.Info("Failed to forward chat message", zap.Error(err))
				return err
			}
		case <-sendPingCh:
			log.Log(minLogLevel, "Sending ping to client")

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
				log.Log(minLogLevel, "Stream is unhealthy; aborting")
				return status.Error(codes.Aborted, "terminating unhealthy stream")
			}
		case <-streamHealthCh:
			log.Log(minLogLevel, "Stream is unhealthy; aborting")
			return status.Error(codes.Aborted, "terminating unhealthy stream")
		case <-ctx.Done():
			log.Log(minLogLevel, "Stream context cancelled; ending stream")
			return status.Error(codes.Canceled, "")
		}
	}
}

func (s *Server) GetMessage(ctx context.Context, req *messagingpb.GetMessageRequest) (*messagingpb.GetMessageResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	allow, _, err := s.rpcAuthz.CanGetMessage(ctx, req.ChatId, userID)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to do rpc authz checks")
	} else if !allow {
		return &messagingpb.GetMessageResponse{Result: messagingpb.GetMessageResponse_DENIED}, nil
	}

	message, err := s.messages.GetMessage(ctx, req.ChatId, req.MessageId)
	if errors.Is(err, ErrMessageNotFound) {
		return &messagingpb.GetMessageResponse{Result: messagingpb.GetMessageResponse_NOT_FOUND}, nil
	} else if err != nil {
		s.log.Error("Failed to get message", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get message")
	}

	return &messagingpb.GetMessageResponse{
		Message: message,
	}, nil
}

func (s *Server) GetMessages(ctx context.Context, req *messagingpb.GetMessagesRequest) (*messagingpb.GetMessagesResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	allow, _, err := s.rpcAuthz.CanGetMessages(ctx, req.ChatId, userID)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to do rpc authz checks")
	} else if !allow {
		return &messagingpb.GetMessagesResponse{Result: messagingpb.GetMessagesResponse_DENIED}, nil
	}

	var messages []*messagingpb.Message
	if req.GetMessageIds() != nil {
		messages, err = s.messages.GetBatchMessages(ctx, req.ChatId, req.GetMessageIds().MessageIds...)
		if err != nil {
			s.log.Error("Failed to get messages by message id batch", zap.Error(err))
			return nil, status.Error(codes.Internal, "failed to get messages")
		}
	} else {
		messages, err = s.messages.GetPagedMessages(ctx, req.ChatId, query.FromProtoOptions(req.GetOptions())...)
		if err != nil {
			s.log.Error("Failed to get messages by query options", zap.Error(err))
			return nil, status.Error(codes.Internal, "failed to get messages")
		}
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

	log := s.log.With(
		zap.String("chat_id", base64.StdEncoding.EncodeToString(req.ChatId.Value)),
		zap.String("user_id", model.UserIDString(userID)),
	)

	allow, reason, err := s.rpcAuthz.CanSendMessage(ctx, req.ChatId, userID, req.Content[0], req.PaymentIntent)
	if err != nil {
		log.Warn("Failed to do rpc authz checks", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to do rpc authz checks")
	} else if !allow {
		log.Info("Not allowed", zap.String("reason", reason))
		return &messagingpb.SendMessageResponse{Result: messagingpb.SendMessageResponse_DENIED}, nil
	}

	// Using chats store causes a import cycle, so we use the heuristic of needing
	// a payment for user-generated content as the on/off stage check.
	//
	// todo: This works for now as an efficient check, but might not in the future.
	var wasSenderOffStage bool
	if req.PaymentIntent != nil {
		switch req.Content[0].Type.(type) {
		case *messagingpb.Content_Text, *messagingpb.Content_Reply:
			wasSenderOffStage = true
		}
	}

	msg := &messagingpb.Message{
		SenderId:          userID,
		Content:           req.Content,
		WasSenderOffStage: wasSenderOffStage,
	}

	sent, err := s.Send(ctx, req.ChatId, msg)
	if err != nil {
		log.Warn("Failed to send message", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to send message")
	}

	// todo: put this logic in a DB transaction alongside message send
	if req.PaymentIntent != nil {
		err = s.intents.MarkFulfilled(ctx, req.PaymentIntent)
		if err == intent.ErrAlreadyFulfilled {
			return &messagingpb.SendMessageResponse{Result: messagingpb.SendMessageResponse_DENIED}, nil
		} else if err != nil {
			log.Warn("Failed to mark intent as fulfilled", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to mark intent as fulfilled")
		}
	}

	return &messagingpb.SendMessageResponse{
		Message: sent,
	}, nil
}

func (s *Server) Send(ctx context.Context, chatID *commonpb.ChatId, msg *messagingpb.Message) (*messagingpb.Message, error) {
	created, err := s.messages.PutMessage(ctx, chatID, msg)
	if err != nil {
		s.log.Error("Failed to put chat message", zap.Error(err))
		return nil, err
	}

	if err := s.eventBus.OnEvent(chatID, &event.ChatEvent{ChatID: chatID, MessageUpdate: created}); err != nil {
		s.log.Warn("Failed to notify event bus", zap.Error(err))
	}

	return created, nil
}

func (s *Server) AdvancePointer(ctx context.Context, req *messagingpb.AdvancePointerRequest) (*messagingpb.AdvancePointerResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	allow, _, err := s.rpcAuthz.CanAdvancePointer(ctx, req.ChatId, userID)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to do rpc authz checks")
	} else if !allow {
		return &messagingpb.AdvancePointerResponse{Result: messagingpb.AdvancePointerResponse_DENIED}, nil
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

	allow, _, err := s.rpcAuthz.CanNotifyIsTyping(ctx, req.ChatId, userID)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to do rpc authz checks")
	} else if !allow {
		return &messagingpb.NotifyIsTypingResponse{Result: messagingpb.NotifyIsTypingResponse_DENIED}, nil
	}

	isTyping := &messagingpb.IsTyping{
		UserId:   userID,
		IsTyping: req.IsTyping,
	}

	if err = s.eventBus.OnEvent(req.ChatId, &event.ChatEvent{ChatID: req.ChatId, IsTyping: isTyping}); err != nil {
		s.log.Warn("Failed to notify event bus", zap.Error(err))
	}

	return &messagingpb.NotifyIsTypingResponse{}, nil
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
