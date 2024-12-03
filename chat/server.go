package chat

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
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

	codedata "github.com/code-payments/code-server/pkg/code/data"
	codekin "github.com/code-payments/code-server/pkg/kin"

	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/event"
	"github.com/code-payments/flipchat-server/intent"
	"github.com/code-payments/flipchat-server/messaging"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/profile"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/query"
)

const (
	StreamBufferSize = 64
	StreamPingDelay  = 5 * time.Second
	StreamTimeout    = time.Second

	MaxFlushedChatBatchSize = 1024
	FlushedChatBatchSize    = 32
)

var (
	InitialCoverCharge = codekin.ToQuarks(100)
	MaxUnreadCount     = uint32(99)
)

type Server struct {
	log      *zap.Logger
	authz    auth.Authorizer
	eventBus *event.Bus[*commonpb.ChatId, *event.ChatEvent]

	chats    Store
	profiles profile.Store
	messages messaging.MessageStore
	pointers messaging.PointerStore
	intents  intent.Store
	codeData codedata.Provider

	messenger messaging.Messenger

	streamsMu sync.RWMutex
	streams   map[string]event.Stream[[]*event.ChatEvent]

	chatpb.UnimplementedChatServer
}

func NewServer(
	log *zap.Logger,
	authz auth.Authorizer,
	chats Store,
	profiles profile.Store,
	messages messaging.MessageStore,
	pointers messaging.PointerStore,
	intents intent.Store,
	codeData codedata.Provider,
	messenger messaging.Messenger,
	eventBus *event.Bus[*commonpb.ChatId, *event.ChatEvent],
) *Server {
	s := &Server{
		log:      log,
		authz:    authz,
		eventBus: eventBus,

		chats:    chats,
		profiles: profiles,
		pointers: pointers,
		messages: messages,
		intents:  intents,
		codeData: codeData,

		messenger: messenger,

		streams: make(map[string]event.Stream[[]*event.ChatEvent]),
	}

	eventBus.AddHandler(event.HandlerFunc[*commonpb.ChatId, *event.ChatEvent](s.OnChatEvent))

	return s
}

func (s *Server) StreamChatEvents(stream grpc.BidiStreamingServer[chatpb.StreamChatEventsRequest, chatpb.StreamChatEventsResponse]) error {
	ctx := stream.Context()

	req, err := protoutil.BoundedReceive[chatpb.StreamChatEventsRequest, *chatpb.StreamChatEventsRequest](
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

	log := s.log.With(zap.String("user_id", model.UserIDString(userID)))
	userKey := string(userID.Value)

	s.streamsMu.Lock()
	if existing, exists := s.streams[userKey]; exists {
		delete(s.streams, userKey)
		existing.Close()

		log.Info("Closed previous stream")
	}

	ss := event.NewProtoEventStream[[]*event.ChatEvent, *chatpb.StreamChatEventsResponse_EventBatch](
		userKey,
		StreamBufferSize,
		func(events []*event.ChatEvent) (*chatpb.StreamChatEventsResponse_EventBatch, bool) {
			if len(events) > MaxFlushedChatBatchSize {
				log.Warn("Chat event batch size exceeds proto limit")
				return nil, false
			}

			var updates []*chatpb.StreamChatEventsResponse_ChatUpdate
			for _, e := range events {
				isMember, err := s.chats.IsMember(ctx, e.ChatID, userID)
				if err != nil {
					log.Warn("Failed to check membership for event, dropping")
					return nil, false
				}

				if !isMember {
					continue
				}

				clonedEvent := e.Clone()

				update := &chatpb.StreamChatEventsResponse_ChatUpdate{
					ChatId:       clonedEvent.ChatID,
					Metadata:     clonedEvent.ChatUpdate,
					MemberUpdate: clonedEvent.MemberUpdate,
					LastMessage:  clonedEvent.MessageUpdate,
					Pointer:      clonedEvent.PointerUpdate,
					IsTyping:     clonedEvent.IsTyping,
				}

				// todo: this section needs tests
				if update.LastMessage != nil {
					// Notably, to propagate unread counts after sending a message
					//
					// todo: Should we have updates for a limited view of a chat so we don't need to fetch entire metadata?
					md, err := s.getMetadata(ctx, e.ChatID, userID)
					if err == nil {
						update.Metadata = md
					} else {
						log.Warn("Failed to get metadata", zap.Error(err))
					}
				}

				if refresh := update.GetMemberUpdate().GetRefresh(); refresh != nil {
					for _, m := range refresh.Members {
						m.IsSelf = bytes.Equal(m.UserId.Value, userID.Value)
					}
				}

				updates = append(updates, update)
			}

			if len(updates) == 0 {
				return nil, false
			}
			return &chatpb.StreamChatEventsResponse_EventBatch{
				Updates: updates,
			}, true
		},
	)

	log = log.With(zap.String("ss", fmt.Sprintf("%p", ss)))
	log.Debug("Initializing stream")

	s.streams[userKey] = ss
	s.streamsMu.Unlock()

	defer func() {
		s.streamsMu.Lock()

		log.Debug("Closing streamer")

		// We check to see if the current active stream is the one that we created.
		// If it is, we can just remove it since it's closed. Otherwise, we leave it
		// be, as another StreamChatEvents() call is handling it.
		liveStream := s.streams[userKey]
		if liveStream == ss {
			delete(s.streams, userKey)
		}

		s.streamsMu.Unlock()
	}()

	sendPingCh := time.After(0)
	streamHealthCh := protoutil.MonitorStreamHealth(ctx, log, stream, func(t *messagingpb.StreamMessagesRequest) bool {
		return t.GetPong() != nil
	})

	go s.flushInitialState(ctx, userID, ss)

	for {
		select {
		case batch, ok := <-ss.Channel():
			if !ok {
				log.Debug("stream closed; ending stream")
				return status.Error(codes.Aborted, "stream closed")
			}

			err = stream.Send(&chatpb.StreamChatEventsResponse{
				Type: &chatpb.StreamChatEventsResponse_Events{
					Events: batch,
				},
			})
			if err != nil {
				log.Info("Failed to forward chat message", zap.Error(err))
				return err
			}
		case <-sendPingCh:
			log.Debug("sending ping to client")

			sendPingCh = time.After(StreamPingDelay)

			err := stream.Send(&chatpb.StreamChatEventsResponse{
				Type: &chatpb.StreamChatEventsResponse_Ping{
					Ping: &commonpb.ServerPing{
						Timestamp: timestamppb.Now(),
						PingDelay: durationpb.New(StreamPingDelay),
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

func (s *Server) GetChats(ctx context.Context, req *chatpb.GetChatsRequest) (*chatpb.GetChatsResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(zap.String("user_id", model.UserIDString(userID)))

	// TODO: Pagination, it's fine for now(!!)
	chatIDs, err := s.chats.GetChatsForUser(ctx, userID)
	if err != nil {
		log.Warn("Failed to get chats", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chats")
	}

	metadata := make([]*chatpb.Metadata, 0, len(chatIDs))
	for _, chatID := range chatIDs {
		md, err := s.getMetadata(ctx, chatID, userID)
		if err != nil {
			log.Warn("Failed to get metadata", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to get metadata")
		}

		metadata = append(metadata, md)
	}

	return &chatpb.GetChatsResponse{
		Chats: metadata,
	}, nil
}

func (s *Server) GetChat(ctx context.Context, req *chatpb.GetChatRequest) (*chatpb.GetChatResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	// TODO: Auth

	var chatID *commonpb.ChatId
	switch t := req.Identifier.(type) {
	case *chatpb.GetChatRequest_ChatId:
		chatID = t.ChatId
	case *chatpb.GetChatRequest_RoomNumber:
		chatID, err = s.chats.GetChatID(ctx, t.RoomNumber)
		if errors.Is(err, ErrChatNotFound) {
			return &chatpb.GetChatResponse{Result: chatpb.GetChatResponse_NOT_FOUND}, nil
		} else if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get chat")
		}
	}

	md, members, err := s.getMetadataWithMembers(ctx, chatID, userID)
	if errors.Is(err, ErrChatNotFound) {
		return &chatpb.GetChatResponse{Result: chatpb.GetChatResponse_NOT_FOUND}, nil
	} else if err != nil {
		s.log.Warn("Failed to get data", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat data")
	}

	return &chatpb.GetChatResponse{
		Metadata: md,
		Members:  members,
	}, nil
}

func (s *Server) StartChat(ctx context.Context, req *chatpb.StartChatRequest) (*chatpb.StartChatResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	var md *chatpb.Metadata
	var users []*commonpb.UserId

	var paymentIntent *commonpb.IntentId
	switch t := req.Parameters.(type) {
	case *chatpb.StartChatRequest_TwoWayChat:
		md = &chatpb.Metadata{
			ChatId: model.MustGenerateTwoWayChatID(userID, t.TwoWayChat.OtherUserId),
			Type:   chatpb.Metadata_TWO_WAY,
		}
		users = []*commonpb.UserId{userID, t.TwoWayChat.OtherUserId}

	case *chatpb.StartChatRequest_GroupChat:
		if t.GroupChat.PaymentIntent == nil {
			return &chatpb.StartChatResponse{Result: chatpb.StartChatResponse_DENIED}, nil
		}

		paymentIntent = t.GroupChat.PaymentIntent

		isFulfilled, err := s.intents.IsFulfilled(ctx, paymentIntent)
		if err != nil {
			s.log.Warn("Failed to check if intent is already fulfilled", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to check if intent is already fulfilled")
		} else if isFulfilled {
			return &chatpb.StartChatResponse{Result: chatpb.StartChatResponse_DENIED}, nil
		}

		var paymentMetadata chatpb.StartGroupChatPaymentMetadata
		err = intent.LoadPaymentMetadata(ctx, s.codeData, t.GroupChat.PaymentIntent, &paymentMetadata)
		if err == intent.ErrNoPaymentMetadata {
			return &chatpb.StartChatResponse{Result: chatpb.StartChatResponse_DENIED}, nil
		} else if err != nil {
			s.log.Warn("Failed to get payment metadata", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to lookup payment metadata")
		}

		// Verify the provided payment is for this user to create a new group.
		if !bytes.Equal(paymentMetadata.UserId.Value, userID.Value) {
			return &chatpb.StartChatResponse{Result: chatpb.StartChatResponse_DENIED}, nil
		}

		// Need to do this transactionally...but we've lost it...so...heh :)
		md = &chatpb.Metadata{
			ChatId:      model.MustGenerateChatID(),
			Type:        chatpb.Metadata_GROUP,
			Title:       t.GroupChat.Title,
			Owner:       userID,
			CoverCharge: &commonpb.PaymentAmount{Quarks: InitialCoverCharge},
		}

		users = append(t.GroupChat.Users, userID)

	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported type")
	}

	md.LastActivity = timestamppb.Now()

	md, err = s.chats.CreateChat(ctx, md)
	if err != nil && !errors.Is(err, ErrChatExists) {
		s.log.Warn("Failed to put chat metadata", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to put chat")
	}

	// Push state is not persisted in the chat metadata
	md.IsPushEnabled = true
	md.CanDisablePush = true

	// todo: put this logic in a DB transaction alongside chat creation
	if paymentIntent != nil {
		err = s.intents.MarkFulfilled(ctx, paymentIntent)
		if err == intent.ErrAlreadyFulfilled {
			return &chatpb.StartChatResponse{Result: chatpb.StartChatResponse_DENIED}, nil
		} else if err != nil {
			s.log.Warn("Failed to mark intent as fulfilled", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to mark intent as fulfilled")
		}
	}

	log := s.log.With(
		zap.String("chat_id", base64.StdEncoding.EncodeToString(md.ChatId.Value)),
		zap.String("user_id", model.UserIDString(userID)),
	)

	if md.Type == chatpb.Metadata_GROUP {
		if err := messaging.SendAnnouncement(
			ctx,
			s.messenger,
			md.ChatId,
			messaging.NewRoomIsLiveAnnouncementContentBuilder(md.RoomNumber),
		); err != nil {
			log.Warn("Failed to send announcement", zap.Error(err))
		}
	}

	var memberProtos []*chatpb.Member
	for _, m := range users {
		member := Member{UserID: m, AddedBy: userID}
		if req.GetGroupChat() != nil && bytes.Equal(m.Value, userID.Value) {
			member.HasModPermission = true
		}

		memberProtos = append(memberProtos, member.ToProto(userID))

		err = s.chats.AddMember(ctx, md.ChatId, member)
		if errors.Is(err, ErrMemberExists) {
			continue
		} else if err != nil {
			log.Warn("Failed to put chat member", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to put chat member")
		}
	}

	var mu *chatpb.StreamChatEventsResponse_MemberUpdate
	if err := s.populateMemberData(ctx, memberProtos, nil); err != nil {
		log.Warn("Failed to get member profiles for notification, not including")
	} else {
		mu = &chatpb.StreamChatEventsResponse_MemberUpdate{
			Kind: &chatpb.StreamChatEventsResponse_MemberUpdate_Refresh_{
				Refresh: &chatpb.StreamChatEventsResponse_MemberUpdate_Refresh{
					Members: memberProtos,
				},
			},
		}
	}

	if err = s.eventBus.OnEvent(md.ChatId, &event.ChatEvent{ChatID: md.ChatId, ChatUpdate: md, MemberUpdate: mu}); err != nil {
		log.Warn("Failed to notify new chat", zap.Error(err))
	}

	return &chatpb.StartChatResponse{Chat: md, Members: memberProtos}, nil
}

func (s *Server) JoinChat(ctx context.Context, req *chatpb.JoinChatRequest) (*chatpb.JoinChatResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	hasPaymentIntent := req.PaymentIntent != nil
	var paymentMetadata chatpb.JoinChatPaymentMetadata

	if hasPaymentIntent {
		isFulfilled, err := s.intents.IsFulfilled(ctx, req.PaymentIntent)
		if err != nil {
			s.log.Warn("Failed to check if intent is already fulfilled", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to check if intent is already fulfilled")
		} else if isFulfilled {
			return &chatpb.JoinChatResponse{Result: chatpb.JoinChatResponse_DENIED}, nil
		}

		err = intent.LoadPaymentMetadata(ctx, s.codeData, req.PaymentIntent, &paymentMetadata)
		if err == intent.ErrNoPaymentMetadata {
			return &chatpb.JoinChatResponse{Result: chatpb.JoinChatResponse_DENIED}, nil
		} else if err != nil {
			s.log.Warn("Failed to get payment metadata", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to lookup payment metadata")
		}
	}

	var chatID *commonpb.ChatId
	switch t := req.Identifier.(type) {
	case *chatpb.JoinChatRequest_ChatId:
		chatID = t.ChatId
	case *chatpb.JoinChatRequest_RoomId:
		chatID, err = s.chats.GetChatID(ctx, t.RoomId)
		if errors.Is(err, ErrChatNotFound) {
			return &chatpb.JoinChatResponse{Result: chatpb.JoinChatResponse_DENIED}, nil
		} else if err != nil {
			s.log.Warn("Failed to get room", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to lookup room")
		}
	}

	if hasPaymentIntent {
		// Verify the provided payment is for this user joining the specified
		// chat.
		if !bytes.Equal(paymentMetadata.UserId.Value, userID.Value) {
			return &chatpb.JoinChatResponse{Result: chatpb.JoinChatResponse_DENIED}, nil
		}
		if !bytes.Equal(paymentMetadata.ChatId.Value, chatID.Value) {
			return &chatpb.JoinChatResponse{Result: chatpb.JoinChatResponse_DENIED}, nil
		}
	} else {
		chatMetadata, err := s.getMetadata(ctx, chatID, nil)
		if err != nil {
			s.log.Warn("Failed to get chat", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to get chat")
		}

		// Only the owner of the chat can join without payment
		if chatMetadata.Owner == nil || !bytes.Equal(chatMetadata.Owner.Value, userID.Value) {
			return &chatpb.JoinChatResponse{Result: chatpb.JoinChatResponse_DENIED}, nil
		}
	}

	// TODO: Auth
	// TODO: Return if no-op

	if err = s.chats.AddMember(ctx, chatID, Member{UserID: userID}); err != nil {
		s.log.Warn("Failed to put chat member", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to put chat member")
	}

	// todo: put this logic in a DB transaction alongside member add
	if hasPaymentIntent {
		err = s.intents.MarkFulfilled(ctx, req.PaymentIntent)
		if err == intent.ErrAlreadyFulfilled {
			return &chatpb.JoinChatResponse{Result: chatpb.JoinChatResponse_DENIED}, nil
		} else if err != nil {
			s.log.Warn("Failed to mark intent as fulfilled", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to mark intent as fulfilled")
		}
	}

	if err := messaging.SendAnnouncement(
		ctx,
		s.messenger,
		chatID,
		messaging.NewUserJoinedChatAnnouncementContentBuilder(ctx, s.profiles, userID),
	); err != nil {
		s.log.Warn("Failed to send announcement", zap.Error(err))
	}

	md, members, err := s.getMetadataWithMembers(ctx, chatID, userID)
	if err != nil {
		s.log.Warn("Failed to get chat data", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat data")
	}

	mu := &chatpb.StreamChatEventsResponse_MemberUpdate{
		Kind: &chatpb.StreamChatEventsResponse_MemberUpdate_Refresh_{
			Refresh: &chatpb.StreamChatEventsResponse_MemberUpdate_Refresh{
				Members: members,
			},
		},
	}

	err = s.eventBus.OnEvent(md.ChatId, &event.ChatEvent{ChatID: md.ChatId, ChatUpdate: md, MemberUpdate: mu})
	if err != nil {
		s.log.Warn("Failed to notify joined member", zap.String("chat_id", base64.StdEncoding.EncodeToString(md.ChatId.Value)), zap.Error(err))
	}

	return &chatpb.JoinChatResponse{Metadata: md, Members: members}, nil
}

func (s *Server) LeaveChat(ctx context.Context, req *chatpb.LeaveChatRequest) (*chatpb.LeaveChatResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("chat_id", base64.StdEncoding.EncodeToString(req.ChatId.Value)),
	)

	if err = s.chats.RemoveMember(ctx, req.ChatId, userID); err != nil {
		log.Warn("Failed to remove member", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to remove chat member")
	}

	md, members, err := s.getMetadataWithMembers(ctx, req.ChatId, userID)
	if err != nil {
		log.Warn("Failed to get chat data for update", zap.Error(err))
		return &chatpb.LeaveChatResponse{}, nil
	}

	mu := &chatpb.StreamChatEventsResponse_MemberUpdate{
		Kind: &chatpb.StreamChatEventsResponse_MemberUpdate_Refresh_{
			Refresh: &chatpb.StreamChatEventsResponse_MemberUpdate_Refresh{
				Members: members,
			},
		},
	}

	err = s.eventBus.OnEvent(md.ChatId, &event.ChatEvent{ChatID: md.ChatId, MemberUpdate: mu})
	if err != nil {
		s.log.Warn("Failed to notify member leaving", zap.String("chat_id", base64.StdEncoding.EncodeToString(md.ChatId.Value)), zap.Error(err))
	}

	return &chatpb.LeaveChatResponse{}, nil
}

func (s *Server) SetCoverCharge(ctx context.Context, req *chatpb.SetCoverChargeRequest) (*chatpb.SetCoverChargeResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("chat_id", base64.StdEncoding.EncodeToString(req.ChatId.Value)),
	)

	md, err := s.getMetadata(ctx, req.ChatId, nil)
	if err == ErrChatNotFound {
		return &chatpb.SetCoverChargeResponse{Result: chatpb.SetCoverChargeResponse_DENIED}, nil
	} else if err != nil {
		log.Warn("Failed to get chat", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat")
	}

	if md.Owner == nil || !bytes.Equal(md.Owner.Value, userID.Value) {
		return &chatpb.SetCoverChargeResponse{Result: chatpb.SetCoverChargeResponse_DENIED}, nil
	}
	if md.CoverCharge == nil {
		return &chatpb.SetCoverChargeResponse{Result: chatpb.SetCoverChargeResponse_CANT_SET}, nil
	}

	err = s.chats.SetCoverCharge(ctx, req.ChatId, req.CoverCharge)
	if err != nil {
		log.Warn("Failed to set cover charge", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to set cover charge")
	}

	if err = messaging.SendAnnouncement(
		ctx,
		s.messenger,
		req.ChatId,
		messaging.NewCoverChangedAnnouncementContentBuilder(req.CoverCharge.Quarks),
	); err != nil {
		log.Warn("Failed to send announcement", zap.Error(err))
	}

	return &chatpb.SetCoverChargeResponse{}, nil
}

func (s *Server) RemoveUser(ctx context.Context, req *chatpb.RemoveUserRequest) (*chatpb.RemoveUserResponse, error) {
	return &chatpb.RemoveUserResponse{Result: chatpb.RemoveUserResponse_DENIED}, nil

	/*
		ownerID, err := s.authz.Authorize(ctx, req, &req.Auth)
		if err != nil {
			return nil, err
		}

		log := s.log.With(
			zap.String("owner_id", model.UserIDString(ownerID)),
			zap.String("user_id", model.UserIDString(req.UserId)),
			zap.String("chat_id", base64.StdEncoding.EncodeToString(req.ChatId.Value)),
		)

		if bytes.Equal(ownerID.Value, req.UserId.Value) {
			return &chatpb.RemoveUserResponse{Result: chatpb.RemoveUserResponse_DENIED}, nil
		}

		md, err := s.getMetadata(ctx, req.ChatId, nil)
		if err == ErrChatNotFound {
			return &chatpb.RemoveUserResponse{Result: chatpb.RemoveUserResponse_DENIED}, nil
		} else if err != nil {
			log.Warn("Failed to get chat data", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to get chat data")
		}

		if md.Owner == nil || !bytes.Equal(md.Owner.Value, ownerID.Value) {
			return &chatpb.RemoveUserResponse{Result: chatpb.RemoveUserResponse_DENIED}, nil
		}

		// todo: Return if no-op

		if err = s.chats.RemoveMember(ctx, req.ChatId, req.UserId); err != nil {
			log.Warn("Failed to remove member", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to remove chat member")
		}

		if err = messaging.SendAnnouncement(
			ctx,
			s.messenger,
			req.ChatId,
			messaging.NewUserRemovedAnnouncementContentBuilder(ctx, s.profiles, req.UserId),
		); err != nil {
			log.Warn("Failed to send announcement", zap.Error(err))
		}

		_, members, err := s.getMetadataWithMembers(ctx, req.ChatId, nil)
		if err != nil {
			log.Warn("Failed to get chat data", zap.Error(err))
			return &chatpb.RemoveUserResponse{}, nil
		}

		mu := &chatpb.StreamChatEventsResponse_MemberUpdate{
			Kind: &chatpb.StreamChatEventsResponse_MemberUpdate_Refresh_{
				Refresh: &chatpb.StreamChatEventsResponse_MemberUpdate_Refresh{
					Members: members,
				},
			},
		}

		err = s.eventBus.OnEvent(md.ChatId, &event.ChatEvent{ChatID: md.ChatId, MemberUpdate: mu})
		if err != nil {
			s.log.Warn("Failed to notify removed member", zap.String("chat_id", base64.StdEncoding.EncodeToString(md.ChatId.Value)), zap.Error(err))
		}

		return &chatpb.RemoveUserResponse{}, nil
	*/
}

// todo: this RPC needs tests
func (s *Server) MuteUser(ctx context.Context, req *chatpb.MuteUserRequest) (*chatpb.MuteUserResponse, error) {
	ownerID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("owner_id", model.UserIDString(ownerID)),
		zap.String("user_id", model.UserIDString(req.UserId)),
		zap.String("chat_id", base64.StdEncoding.EncodeToString(req.ChatId.Value)),
	)

	if bytes.Equal(ownerID.Value, req.UserId.Value) {
		return &chatpb.MuteUserResponse{Result: chatpb.MuteUserResponse_DENIED}, nil
	}

	md, err := s.getMetadata(ctx, req.ChatId, nil)
	if err == ErrChatNotFound {
		return &chatpb.MuteUserResponse{Result: chatpb.MuteUserResponse_DENIED}, nil
	} else if err != nil {
		log.Warn("Failed to get chat data", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat data")
	}

	if md.Owner == nil || !bytes.Equal(md.Owner.Value, ownerID.Value) {
		return &chatpb.MuteUserResponse{Result: chatpb.MuteUserResponse_DENIED}, nil
	}

	// todo: Return if no-op

	if err = s.chats.SetMuteState(ctx, req.ChatId, req.UserId, true); err != nil {
		log.Warn("Failed to mute chat member", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to mute chat member")
	}

	if err = messaging.SendAnnouncement(
		ctx,
		s.messenger,
		req.ChatId,
		messaging.NewUserMutedAnnouncementContentBuilder(ctx, s.profiles, ownerID, req.UserId),
	); err != nil {
		log.Warn("Failed to send announcement", zap.Error(err))
	}

	_, members, err := s.getMetadataWithMembers(ctx, req.ChatId, nil)
	if err != nil {
		log.Warn("Failed to get chat data", zap.Error(err))
		return &chatpb.MuteUserResponse{}, nil
	}

	mu := &chatpb.StreamChatEventsResponse_MemberUpdate{
		Kind: &chatpb.StreamChatEventsResponse_MemberUpdate_Refresh_{
			Refresh: &chatpb.StreamChatEventsResponse_MemberUpdate_Refresh{
				Members: members,
			},
		},
	}

	err = s.eventBus.OnEvent(md.ChatId, &event.ChatEvent{ChatID: md.ChatId, MemberUpdate: mu})
	if err != nil {
		s.log.Warn("Failed to notify muted member", zap.String("chat_id", base64.StdEncoding.EncodeToString(md.ChatId.Value)), zap.Error(err))
	}

	return &chatpb.MuteUserResponse{}, nil
}

// todo: this RPC needs tests
func (s *Server) MuteChat(ctx context.Context, req *chatpb.MuteChatRequest) (*chatpb.MuteChatResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("chat_id", base64.StdEncoding.EncodeToString(req.ChatId.Value)),
	)

	isMember, err := s.chats.IsMember(ctx, req.ChatId, userID)
	if err != nil {
		log.Warn("Failed to get chat membership", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat membership")
	} else if !isMember {
		return &chatpb.MuteChatResponse{Result: chatpb.MuteChatResponse_DENIED}, nil
	}

	if err = s.chats.SetPushState(ctx, req.ChatId, userID, false); err != nil {
		log.Warn("Failed to mute chat", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to mute chat")
	}

	return &chatpb.MuteChatResponse{}, nil
}

// todo: this RPC needs tests
func (s *Server) UnmuteChat(ctx context.Context, req *chatpb.UnmuteChatRequest) (*chatpb.UnmuteChatResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("chat_id", base64.StdEncoding.EncodeToString(req.ChatId.Value)),
	)

	isMember, err := s.chats.IsMember(ctx, req.ChatId, userID)
	if err != nil {
		log.Warn("Failed to get chat membership", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat membership")
	} else if !isMember {
		return &chatpb.UnmuteChatResponse{Result: chatpb.UnmuteChatResponse_DENIED}, nil
	}

	if err = s.chats.SetPushState(ctx, req.ChatId, userID, true); err != nil {
		log.Warn("Failed to mute chat", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to unmute chat")
	}

	return &chatpb.UnmuteChatResponse{}, nil
}

// todo: implement me
func (s *Server) ReportUser(ctx context.Context, req *chatpb.ReportUserRequest) (*chatpb.ReportUserResponse, error) {
	return &chatpb.ReportUserResponse{}, nil
}

func (s *Server) OnChatEvent(chatID *commonpb.ChatId, e *event.ChatEvent) {
	clonedEvent := e.Clone()

	members, err := s.chats.GetMembers(context.Background(), chatID)
	if err != nil {
		s.log.Warn("Failed to get chat members for notification", zap.Error(err))
		return
	}

	// todo: this section needs tests
	if clonedEvent.MessageUpdate != nil {
		err := s.chats.AdvanceLastChatActivity(context.Background(), chatID, clonedEvent.MessageUpdate.Ts.AsTime())
		if err != nil {
			s.log.Warn("Failed to advance chat activity timestamp", zap.Error(err))
		} else if clonedEvent.ChatUpdate != nil {
			clonedEvent.ChatUpdate.LastActivity = clonedEvent.MessageUpdate.Ts
		}
	}

	for _, member := range members {
		s.streamsMu.RLock()
		stream, exists := s.streams[string(member.UserID.Value)]
		s.streamsMu.RUnlock()

		if exists {
			if err = stream.Notify([]*event.ChatEvent{clonedEvent}, StreamTimeout); err != nil {
				s.log.Warn("Failed to send event", zap.Error(err))
			}
		}
	}
}

func (s *Server) getMetadata(ctx context.Context, chatID *commonpb.ChatId, caller *commonpb.UserId) (*chatpb.Metadata, error) {
	md, err := s.chats.GetChatMetadata(ctx, chatID)
	if err != nil {
		return nil, err
	}

	md.CanDisablePush = true

	// If the caller is not specified, _or_ the caller isn't a member, we don't need to fill out
	// caller specific fields.
	if caller == nil {
		return md, nil
	}

	ptrs, err := s.pointers.GetPointers(ctx, chatID, caller)
	if err != nil {
		return nil, fmt.Errorf("failed to get caller pointers: %w", err)
	}

	var rPtr *messagingpb.MessageId
	for _, ptr := range ptrs {
		if ptr.Type == messagingpb.Pointer_READ {
			rPtr = ptr.Value
			break
		}
	}

	// todo: this needs testing
	unread, err := s.messages.CountUnread(ctx, chatID, caller, rPtr, int64(MaxUnreadCount+1))
	if err != nil {
		return nil, fmt.Errorf("failed to count unread messages: %w", err)
	}
	md.NumUnread = uint32(unread)
	if md.NumUnread > MaxUnreadCount {
		md.NumUnread = uint32(MaxUnreadCount)
		md.HasMoreUnread = true
	}

	// todo: this needs testing
	isPushEnabled, err := s.chats.IsPushEnabled(ctx, chatID, caller)
	if err == ErrMemberNotFound {
		isPushEnabled = true
	} else if err != nil {
		return nil, fmt.Errorf("failed to get push state: %w", err)
	}
	md.IsPushEnabled = isPushEnabled

	return md, nil
}

func (s *Server) getMetadataWithMembers(ctx context.Context, chatID *commonpb.ChatId, caller *commonpb.UserId) (*chatpb.Metadata, []*chatpb.Member, error) {
	md, err := s.getMetadata(ctx, chatID, caller)
	if err != nil {
		return nil, nil, err
	}

	members, err := s.getMembers(ctx, md, caller)
	if err != nil {
		return nil, nil, err
	}

	return md, members, nil
}

func (s *Server) getMembers(ctx context.Context, md *chatpb.Metadata, caller *commonpb.UserId) ([]*chatpb.Member, error) {
	members, err := s.chats.GetMembers(ctx, md.ChatId)
	if err == ErrMemberNotFound {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to get members: %w", err)
	}

	memberProtos := make([]*chatpb.Member, 0, len(members))
	for _, member := range members {
		p := member.ToProto(caller)
		if bytes.Equal(member.UserID.Value, md.Owner.GetValue()) {
			p.IsModerator = true
		}

		memberProtos = append(memberProtos, p)
	}

	if err = s.populateMemberData(ctx, memberProtos, md.ChatId); err != nil {
		return nil, fmt.Errorf("failed to populate member data: %w", err)
	}

	return memberProtos, nil
}

func (s *Server) populateMemberData(ctx context.Context, members []*chatpb.Member, chatID *commonpb.ChatId) error {
	for _, m := range members {
		p, err := s.profiles.GetProfile(ctx, m.UserId)
		if err != nil && !errors.Is(err, profile.ErrNotFound) {
			return fmt.Errorf("failed to get user profile: %w", err)
		}
		if err == nil {
			m.Identity = &chatpb.MemberIdentity{
				DisplayName: p.GetDisplayName(),
			}
		}

		if chatID == nil {
			continue
		}

		pointers, err := s.pointers.GetPointers(ctx, chatID, m.UserId)
		if err != nil {
			return fmt.Errorf("failed to get pointers: %w", err)
		}

		m.Pointers = pointers
	}
	return nil
}

func (s *Server) flushInitialState(ctx context.Context, userID *commonpb.UserId, ss event.Stream[[]*event.ChatEvent]) {
	log := s.log.With(zap.String("user_id", model.UserIDString(userID)))

	chatIDs, err := s.chats.GetChatsForUser(ctx, userID)
	switch err {
	case nil:
	case ErrChatNotFound:
		return
	default:
		log.Warn("Failed to get chats for user (stream flush)", zap.Error(err))
		return
	}

	var events []*event.ChatEvent
	for _, chatID := range chatIDs {
		md, err := s.getMetadata(ctx, chatID, userID)
		if err != nil {
			log.Warn("Failed to get metadata for chat (stream flush)", zap.Error(err), zap.String("chat_id", base64.StdEncoding.EncodeToString(chatID.Value)))
			return
		}

		e := &event.ChatEvent{
			ChatID:     chatID,
			ChatUpdate: md,
		}
		events = append(events, e)

		messages, err := s.messages.GetMessages(ctx, e.ChatID, query.WithDescending(), query.WithLimit(1))
		if err != nil {
			log.Warn("Failed to get last message for chat (stream flush)", zap.Error(err), zap.String("chat_id", base64.StdEncoding.EncodeToString(e.ChatID.Value)))
		} else if len(messages) > 0 {
			e.MessageUpdate = messages[len(messages)-1]
		}
	}

	sorted := event.ByLastActivityTimestamp(events)
	sort.Sort(sort.Reverse(sorted))

	var batch []*event.ChatEvent
	for _, e := range sorted {
		batch = append(batch, e)
		if len(batch) >= FlushedChatBatchSize {
			if err = ss.Notify(batch, StreamTimeout); err != nil {
				log.Info("Failed to notify stream (stream flush)", zap.Error(err))
				return
			}
			batch = nil
		}
	}
	if len(batch) > 0 {
		if err = ss.Notify(batch, StreamTimeout); err != nil {
			log.Warn("Failed to notify stream (stream flush)", zap.Error(err))
		}
	}
}
