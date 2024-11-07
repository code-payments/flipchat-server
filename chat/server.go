package chat

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
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

	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/event"
	"github.com/code-payments/flipchat-server/messaging"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/profile"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/query"
)

const (
	streamBufferSize = 64
	streamPingDelay  = 5 * time.Second
	streamTimeout    = time.Second
)

type Server struct {
	log   *zap.Logger
	authz auth.Authorizer

	chats    Store
	profiles profile.Store
	messages messaging.MessageStore
	pointers messaging.PointerStore

	streamsMu sync.RWMutex
	streams   map[string]event.Stream[*event.ChatEvent]

	chatpb.UnimplementedChatServer
}

func NewServer(
	log *zap.Logger,
	authz auth.Authorizer,
	chats Store,
	profiles profile.Store,
	messages messaging.MessageStore,
	pointers messaging.PointerStore,
	eventBus *event.Bus[*commonpb.ChatId, *event.ChatEvent],
) *Server {
	s := &Server{
		log:   log,
		authz: authz,

		chats:    chats,
		profiles: profiles,
		pointers: pointers,
		messages: messages,

		streams: make(map[string]event.Stream[*event.ChatEvent]),
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

	ss := event.NewProtoEventStream[*event.ChatEvent, *chatpb.StreamChatEventsResponse_EventBatch](
		userKey,
		streamBufferSize,
		func(e *event.ChatEvent) (*chatpb.StreamChatEventsResponse_EventBatch, bool) {
			isMember, err := s.chats.IsMember(ctx, e.ChatID, userID)
			if err != nil {
				log.Warn("Failed to check membership for event, dropping")
				return nil, false
			}

			if !isMember {
				return nil, false
			}

			update := &chatpb.StreamChatEventsResponse_ChatUpdate{
				ChatId:       e.ChatID,
				Metadata:     e.ChatUpdate,
				MemberUpdate: e.MemberUpdate,
				LastMessage:  e.MessageUpdate,
				Pointer:      e.PointerUpdate,
				IsTyping:     e.IsTyping,
			}

			return &chatpb.StreamChatEventsResponse_EventBatch{
				Updates: []*chatpb.StreamChatEventsResponse_ChatUpdate{update},
			}, false
		},
	)

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

			sendPingCh = time.After(streamPingDelay)

			err := stream.Send(&chatpb.StreamChatEventsResponse{
				Type: &chatpb.StreamChatEventsResponse_Ping{
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

	switch t := req.Parameters.(type) {
	case *chatpb.StartChatRequest_TwoWayChat:
		md = &chatpb.Metadata{
			ChatId:   model.MustGenerateTwoWayChatID(userID, t.TwoWayChat.OtherUserId),
			Type:     chatpb.Metadata_TWO_WAY,
			Muteable: true,
		}
		users = []*commonpb.UserId{userID, t.TwoWayChat.OtherUserId}

	case *chatpb.StartChatRequest_GroupChat:
		// Need to do this transactionally...but we've lost it...so...heh :)
		md = &chatpb.Metadata{
			ChatId:   model.MustGenerateChatID(),
			Type:     chatpb.Metadata_GROUP,
			Title:    t.GroupChat.Title,
			Muteable: true,
		}

		users = append(t.GroupChat.Users, userID)

	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported type")
	}

	md, err = s.chats.CreateChat(ctx, md)
	if err != nil && !errors.Is(err, ErrChatExists) {
		s.log.Warn("Failed to put chat metadata", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to put chat")
	}

	for _, m := range users {
		member := Member{UserID: m, AddedBy: userID}
		if req.GetGroupChat() != nil && bytes.Equal(m.Value, userID.Value) {
			member.IsHost = true
		}

		err = s.chats.AddMember(ctx, md.ChatId, member)
		if errors.Is(err, ErrMemberExists) {
			continue
		} else if err != nil {
			s.log.Warn("Failed to put chat metadata", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to put chat member")
		}
	}

	return &chatpb.StartChatResponse{Chat: md}, nil
}

func (s *Server) JoinChat(ctx context.Context, req *chatpb.JoinChatRequest) (*chatpb.JoinChatResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
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

	// TODO: Auth

	if err = s.chats.AddMember(ctx, chatID, Member{UserID: userID}); err != nil {
		s.log.Warn("Failed to put chat member", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to put chat member")
	}

	md, members, err := s.getMetadataWithMembers(ctx, chatID, userID)
	if err != nil {
		s.log.Warn("Failed to get chat data", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat data")
	}

	return &chatpb.JoinChatResponse{
		Metadata: md,
		Members:  members,
	}, nil
}

func (s *Server) LeaveChat(ctx context.Context, req *chatpb.LeaveChatRequest) (*chatpb.LeaveChatResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	if err = s.chats.RemoveMember(ctx, req.ChatId, userID); err != nil {
		s.log.Warn("Failed to remove member", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to remove chat member")
	}

	return &chatpb.LeaveChatResponse{}, nil
}

func (s *Server) SetMuteState(ctx context.Context, req *chatpb.SetMuteStateRequest) (*chatpb.SetMuteStateResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	err = s.chats.SetMuteState(ctx, req.ChatId, userID, req.IsMuted)
	if errors.Is(err, ErrMemberNotFound) {
		return &chatpb.SetMuteStateResponse{Result: chatpb.SetMuteStateResponse_DENIED}, nil
	} else if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to set state")
	}

	return &chatpb.SetMuteStateResponse{}, nil
}

func (s *Server) OnChatEvent(chatID *commonpb.ChatId, event *event.ChatEvent) {
	memberIDs, err := s.chats.GetMembers(context.Background(), chatID)
	if err != nil {
		s.log.Warn("Failed to get chat members for notification", zap.Error(err))
		return
	}

	s.streamsMu.RLock()
	defer s.streamsMu.RUnlock()

	for _, memberID := range memberIDs {
		if stream, exists := s.streams[string(memberID.UserID.Value)]; exists {
			if err = stream.Notify(event, streamTimeout); err != nil {
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

	// Need to get participant for isMuted
	member, err := s.chats.GetMember(ctx, chatID, caller)
	if err != nil {
		return nil, nil
	}
	md.IsMuted = member.IsMuted

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

	unread, err := s.messages.CountUnread(ctx, chatID, caller, rPtr)
	if err != nil {
		return nil, fmt.Errorf("failed to count unread messages: %w", err)
	}
	md.NumUnread = uint32(unread)

	return md, nil
}

func (s *Server) getMetadataWithMembers(ctx context.Context, chatID *commonpb.ChatId, caller *commonpb.UserId) (*chatpb.Metadata, []*chatpb.Member, error) {
	md, err := s.chats.GetChatMetadata(ctx, chatID)
	if err != nil {
		return nil, nil, err
	}

	members, err := s.chats.GetMembers(ctx, chatID)
	memberProtos := make([]*chatpb.Member, 0, len(members))
	for _, m := range members {
		mp := &chatpb.Member{
			UserId: m.UserID,
			IsSelf: bytes.Equal(m.UserID.Value, caller.Value),
			IsHost: m.IsHost,
		}

		if mp.IsSelf {
			md.IsMuted = m.IsMuted
		}

		p, err := s.profiles.GetProfile(ctx, m.UserID)
		if err != nil && !errors.Is(err, profile.ErrNotFound) {
			return nil, nil, fmt.Errorf("failed to get user profile: %w", err)
		}
		mp.Identity = &chatpb.MemberIdentity{
			DisplayName: p.GetDisplayName(),
		}

		pointers, err := s.pointers.GetPointers(ctx, chatID, m.UserID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get pointers: %w", err)
		}
		mp.Pointers = pointers

		memberProtos = append(memberProtos, mp)
	}

	return md, memberProtos, nil
}

func (s *Server) flushInitialState(ctx context.Context, userID *commonpb.UserId, ss event.Stream[*event.ChatEvent]) {
	log := s.log.With(zap.String("user_id", model.UserIDString(userID)))

	chatIDs, err := s.chats.GetChatsForUser(ctx, userID)
	if err != nil {
		log.Warn("Failed to get chats for user (steam flush)", zap.Error(err))
		return
	}

	for _, chatID := range chatIDs {
		md, members, err := s.getMetadataWithMembers(ctx, chatID, userID)
		if err != nil {
			log.Warn("Failed to get metadata for chat (steam flush)", zap.Error(err), zap.String("chat_id", base64.StdEncoding.EncodeToString(chatID.Value)))
			continue
		}

		event := &event.ChatEvent{
			ChatID:     chatID,
			ChatUpdate: md,
			MemberUpdate: &chatpb.StreamChatEventsResponse_MemberUpdate{
				Kind: &chatpb.StreamChatEventsResponse_MemberUpdate_Refresh_{
					Refresh: &chatpb.StreamChatEventsResponse_MemberUpdate_Refresh{
						Members: members,
					},
				},
			},
		}

		messages, err := s.messages.GetMessages(ctx, chatID, query.WithDescending(), query.WithLimit(1))
		if err != nil {
			log.Warn("Failed to get last message for chat (steam flush)", zap.Error(err), zap.String("chat_id", base64.StdEncoding.EncodeToString(chatID.Value)))
		}
		if len(messages) > 0 {
			event.MessageUpdate = messages[len(messages)-1]
		}

		if err = ss.Notify(event, streamTimeout); err != nil {
			log.Warn("Failed to notify stream (steam flush)", zap.Error(err))
		}
	}
}
