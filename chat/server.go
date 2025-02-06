package chat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
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
	codekin "github.com/code-payments/code-server/pkg/kin"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/event"
	"github.com/code-payments/flipchat-server/intent"
	"github.com/code-payments/flipchat-server/messaging"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/moderation"
	"github.com/code-payments/flipchat-server/profile"
	"github.com/code-payments/flipchat-server/protoutil"
	"github.com/code-payments/flipchat-server/query"
)

const (
	StreamBufferSize = 64
	StreamPingDelay  = 5 * time.Second
	StreamTimeout    = time.Second

	MaxChatEventBatchSize = 1024
	FlushedChatBatchSize  = 32
)

var (
	InitialMessagingFee = codekin.ToQuarks(100)
	MaxUnreadCount      = uint32(99)
)

type Server struct {
	log      *zap.Logger
	authz    auth.Authorizer
	eventBus *event.Bus[*commonpb.ChatId, *event.ChatEvent]

	accounts account.Store
	chats    Store
	intents  intent.Store
	messages messaging.MessageStore
	pointers messaging.PointerStore
	profiles profile.Store
	codeData codedata.Provider

	messenger messaging.Messenger

	moderationClient moderation.ModerationClient

	streamsMu sync.RWMutex
	streams   map[string]event.Stream[[]*event.ChatEvent]

	chatpb.UnimplementedChatServer
}

func NewServer(
	log *zap.Logger,
	authz auth.Authorizer,
	accounts account.Store,
	chats Store,
	intents intent.Store,
	messages messaging.MessageStore,
	pointers messaging.PointerStore,
	profiles profile.Store,
	codeData codedata.Provider,
	messenger messaging.Messenger,
	moderationClient moderation.ModerationClient,
	eventBus *event.Bus[*commonpb.ChatId, *event.ChatEvent],
) *Server {
	s := &Server{
		log:      log,
		authz:    authz,
		eventBus: eventBus,

		accounts: accounts,
		chats:    chats,
		intents:  intents,
		messages: messages,
		pointers: pointers,
		profiles: profiles,
		codeData: codeData,

		messenger: messenger,

		moderationClient: moderationClient,

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

	minLogLevel := zap.DebugLevel
	isStaff, _ := s.accounts.IsStaff(ctx, userID)
	if isStaff {
		minLogLevel = zap.InfoLevel
	}

	streamID := uuid.New()

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("stream_id", streamID.String()),
	)
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
			if len(events) > MaxChatEventBatchSize {
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
					ChatId:          clonedEvent.ChatID,
					MetadataUpdates: clonedEvent.MetadataUpdates,
					MemberUpdates:   clonedEvent.MemberUpdates,
					LastMessage:     clonedEvent.MessageUpdate,
					Pointer:         clonedEvent.PointerUpdate,
					IsTyping:        clonedEvent.IsTyping,
				}

				// Inject unread count updates specific to this user for a latest message or read pointer update
				var includeUnreadCountUpdate bool
				var readPtr *messagingpb.MessageId
				if update.LastMessage != nil {
					switch update.LastMessage.Content[0].Type.(type) {
					case *messagingpb.Content_Text, *messagingpb.Content_Reply:
						includeUnreadCountUpdate = true
					}
				}
				if update.Pointer != nil && update.Pointer.Pointer.Type == messagingpb.Pointer_READ && bytes.Equal(update.Pointer.Member.Value, userID.Value) {
					includeUnreadCountUpdate = true
					readPtr = update.Pointer.Pointer.Value
				}
				if includeUnreadCountUpdate {
					numUnread, hasMoreUnread, err := s.getUnreadCount(ctx, update.ChatId, userID, readPtr)
					if err == nil {
						// Assumes the full metadata update is only sent on chat creation.
						update.MetadataUpdates = append(
							update.MetadataUpdates,
							&chatpb.MetadataUpdate{
								Kind: &chatpb.MetadataUpdate_UnreadCountChanged_{
									UnreadCountChanged: &chatpb.MetadataUpdate_UnreadCountChanged{
										NumUnread:     numUnread,
										HasMoreUnread: hasMoreUnread,
									},
								},
							},
						)
					} else {
						log.Warn("Failed to get unread count", zap.Error(err))
					}
				}

				// Update all instances of IsSelf for relevant member updates
				for _, memberUpdate := range update.MemberUpdates {
					switch typed := memberUpdate.Kind.(type) {
					case *chatpb.MemberUpdate_FullRefresh_:
						for _, m := range typed.FullRefresh.Members {
							m.IsSelf = bytes.Equal(m.UserId.Value, userID.Value)
						}
					case *chatpb.MemberUpdate_IndividualRefresh_:
						typed.IndividualRefresh.Member.IsSelf = bytes.Equal(typed.IndividualRefresh.Member.UserId.Value, userID.Value)
					case *chatpb.MemberUpdate_Joined_:
						typed.Joined.Member.IsSelf = bytes.Equal(typed.Joined.Member.UserId.Value, userID.Value)
					}
				}
				if fullRefresh := update.GetMemberUpdate().GetFullRefresh(); fullRefresh != nil {
					for _, m := range fullRefresh.Members {
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

	log.Log(minLogLevel, "Initializing stream")

	s.streams[userKey] = ss
	s.streamsMu.Unlock()

	defer func() {
		s.streamsMu.Lock()

		log.Log(minLogLevel, "Closing streamer")

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
				log.Log(minLogLevel, "Stream closed; ending stream")
				return status.Error(codes.Aborted, "stream closed")
			}

			log.Log(minLogLevel, "Forwarding chat update")
			err = stream.Send(&chatpb.StreamChatEventsResponse{
				Type: &chatpb.StreamChatEventsResponse_Events{
					Events: batch,
				},
			})
			if err != nil {
				log.Info("Failed to forward chat update", zap.Error(err))
				return err
			}
		case <-sendPingCh:
			log.Log(minLogLevel, "Sending ping to client")

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

	metadata, err := s.getMetadataBatched(ctx, chatIDs, userID)
	if err != nil {
		log.Warn("Failed to get metadata", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get metadata")
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

	var md *chatpb.Metadata
	var members []*chatpb.Member
	if req.ExcludeMembers {
		md, err = s.getMetadata(ctx, chatID, userID)
		if errors.Is(err, ErrChatNotFound) {
			return &chatpb.GetChatResponse{Result: chatpb.GetChatResponse_NOT_FOUND}, nil
		} else if err != nil {
			s.log.Warn("Failed to get chat metadata", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to get chat metadata")
		}
	} else {
		md, members, err = s.getMetadataWithMembers(ctx, chatID, userID)
		if errors.Is(err, ErrChatNotFound) {
			return &chatpb.GetChatResponse{Result: chatpb.GetChatResponse_NOT_FOUND}, nil
		} else if err != nil {
			s.log.Warn("Failed to get chat metadata with members", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to get chat metadata with members")
		}
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
		_, err = intent.LoadPaymentMetadata(ctx, s.codeData, t.GroupChat.PaymentIntent, &paymentMetadata)
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
			ChatId:       model.MustGenerateChatID(),
			Type:         chatpb.Metadata_GROUP,
			Owner:        userID,
			MessagingFee: &commonpb.PaymentAmount{Quarks: InitialMessagingFee},
		}

		if len(t.GroupChat.DisplayName) > 0 {
			// todo: remember the display name moderation result from the check RPC
			moderationResult, err := s.moderationClient.ClassifyText(t.GroupChat.DisplayName)
			if err != nil {
				s.log.Warn("Failed to moderate display name", zap.Error(err))
				return nil, status.Errorf(codes.Internal, "failed to moderate display name")
			}

			if moderationResult.Flagged {
				return &chatpb.StartChatResponse{Result: chatpb.StartChatResponse_DENIED}, nil
			}

			md.DisplayName = t.GroupChat.DisplayName
		}

		users = append(t.GroupChat.Users, userID)

	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported type")
	}

	md.OpenStatus = &chatpb.OpenStatus{
		IsCurrentlyOpen: true,
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

	var memberProtos []*chatpb.Member
	for _, m := range users {
		member := Member{UserID: m, AddedBy: userID, HasSendPermission: true}
		if req.GetGroupChat() != nil && bytes.Equal(m.Value, userID.Value) {
			member.HasModPermission = true
		}

		memberProtos = append(memberProtos, member.ToProto(nil))

		err = s.chats.AddMember(ctx, md.ChatId, member)
		if errors.Is(err, ErrMemberExists) {
			continue
		} else if err != nil {
			log.Warn("Failed to put chat member", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to put chat member")
		}
	}

	if err := s.populateMemberData(ctx, memberProtos, nil); err != nil {
		log.Warn("Failed to populate additional member data, not including")
	}

	go func() {
		ctx := context.Background()

		if md.Type == chatpb.Metadata_GROUP {
			announcementContentBuilder := messaging.NewRoomIsLiveAnnouncementContentBuilder(md.RoomNumber)
			isStaff, _ := s.accounts.IsStaff(ctx, userID)
			if isStaff {
				announcementContentBuilder = messaging.NewFlipchatIsLiveAnnouncementContentBuilder(md.RoomNumber)
			}

			if err := messaging.SendAnnouncement(
				ctx,
				s.messenger,
				md.ChatId,
				announcementContentBuilder,
			); err != nil {
				log.Warn("Failed to send announcement", zap.Error(err))
			}
		}

		if err = s.eventBus.OnEvent(md.ChatId, &event.ChatEvent{
			ChatID: md.ChatId,
			MetadataUpdates: []*chatpb.MetadataUpdate{
				{
					Kind: &chatpb.MetadataUpdate_FullRefresh_{
						FullRefresh: &chatpb.MetadataUpdate_FullRefresh{
							Metadata: md,
						},
					},
				},
			},
			MemberUpdates: []*chatpb.MemberUpdate{
				{
					Kind: &chatpb.MemberUpdate_FullRefresh_{
						FullRefresh: &chatpb.MemberUpdate_FullRefresh{
							Members: memberProtos,
						},
					},
				},
			},
		}); err != nil {
			log.Warn("Failed to notify new chat", zap.Error(err))
		}
	}()

	// todo: returned members needs testing
	return &chatpb.StartChatResponse{Chat: md, Members: memberProtos}, nil
}

func (s *Server) JoinChat(ctx context.Context, req *chatpb.JoinChatRequest) (*chatpb.JoinChatResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(zap.String("user_id", model.UserIDString(userID)))

	hasPaymentIntent := req.PaymentIntent != nil
	var paymentMetadata chatpb.JoinChatPaymentMetadata

	if hasPaymentIntent {
		if req.WithoutSendPermission {
			log.Warn("Users should not pay for a chat they can't send messages in", zap.Error(err))
			return &chatpb.JoinChatResponse{Result: chatpb.JoinChatResponse_DENIED}, nil
		}

		isFulfilled, err := s.intents.IsFulfilled(ctx, req.PaymentIntent)
		if err != nil {
			log.Warn("Failed to check if intent is already fulfilled", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to check if intent is already fulfilled")
		} else if isFulfilled {
			return &chatpb.JoinChatResponse{Result: chatpb.JoinChatResponse_DENIED}, nil
		}

		_, err = intent.LoadPaymentMetadata(ctx, s.codeData, req.PaymentIntent, &paymentMetadata)
		if err == intent.ErrNoPaymentMetadata {
			return &chatpb.JoinChatResponse{Result: chatpb.JoinChatResponse_DENIED}, nil
		} else if err != nil {
			log.Warn("Failed to get payment metadata", zap.Error(err))
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
			log.Warn("Failed to get room", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to lookup room")
		}
	}

	log = log.With(zap.String("chat_id", base64.StdEncoding.EncodeToString(chatID.Value)))

	chatMetadata, err := s.getMetadata(ctx, chatID, nil)
	if err != nil {
		log.Warn("Failed to get chat", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat")
	}
	isOwner := chatMetadata.Owner != nil && bytes.Equal(chatMetadata.Owner.Value, userID.Value)
	isSpectator := req.WithoutSendPermission

	if hasPaymentIntent {
		// Verify the provided payment is for this user joining the specified
		// chat.
		if !bytes.Equal(paymentMetadata.UserId.Value, userID.Value) {
			return &chatpb.JoinChatResponse{Result: chatpb.JoinChatResponse_DENIED}, nil
		}
		if !bytes.Equal(paymentMetadata.ChatId.Value, chatID.Value) {
			return &chatpb.JoinChatResponse{Result: chatpb.JoinChatResponse_DENIED}, nil
		}
	} else if !isOwner && !isSpectator {
		return &chatpb.JoinChatResponse{Result: chatpb.JoinChatResponse_DENIED}, nil
	}

	// TODO: Return if no-op

	newMember := Member{
		UserID:        userID,
		IsPushEnabled: true,
	}
	if isOwner {
		newMember.HasSendPermission = true
		newMember.HasModPermission = true
	}

	// todo: put this logic in a DB transaction alongside member add
	if hasPaymentIntent {
		newMember.HasSendPermission = true

		isMember, err := s.chats.IsMember(ctx, chatID, userID)
		if err != nil {
			log.Warn("Failed to check if user is already a member", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to check if user is already a member")
		}

		if isMember {
			err := s.chats.SetSendPermission(ctx, chatID, userID, true)
			if err != nil {
				log.Warn("Failed to set send permission", zap.Error(err))
				return nil, status.Errorf(codes.Internal, "failed to set send permission")
			}
		} else {
			if err = s.chats.AddMember(ctx, chatID, newMember); err != nil {
				log.Warn("Failed to put chat member", zap.Error(err))
				return nil, status.Errorf(codes.Internal, "failed to put chat member")
			}
		}

		err = s.intents.MarkFulfilled(ctx, req.PaymentIntent)
		if err == intent.ErrAlreadyFulfilled {
			return &chatpb.JoinChatResponse{Result: chatpb.JoinChatResponse_DENIED}, nil
		} else if err != nil {
			s.log.Warn("Failed to mark intent as fulfilled", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to mark intent as fulfilled")
		}

	} else {

		if err = s.chats.AddMember(ctx, chatID, newMember); err != nil {
			log.Warn("Failed to put chat spectator", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to put chat member")
		}

	}

	md, members, err := s.getMetadataWithMembers(ctx, chatID, userID)
	if err != nil {
		log.Warn("Failed to get chat data", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat data")
	}

	go func() {
		ctx := context.Background()

		protoMember := newMember.ToProto(nil)
		err = s.populateMemberData(ctx, []*chatpb.Member{protoMember}, nil)
		if err != nil {
			log.Warn("failed to populate additional member data", zap.Error(err))
		}

		err = s.eventBus.OnEvent(chatID, &event.ChatEvent{ChatID: chatID, MemberUpdates: []*chatpb.MemberUpdate{
			{
				Kind: &chatpb.MemberUpdate_Joined_{
					Joined: &chatpb.MemberUpdate_Joined{
						Member: protoMember,
					},
				},
			},
		}})
		if err != nil {
			log.Warn("Failed to notify joined member", zap.Error(err))
		}
	}()

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

	err = s.eventBus.OnEvent(req.ChatId, &event.ChatEvent{ChatID: req.ChatId, MemberUpdates: []*chatpb.MemberUpdate{
		{
			Kind: &chatpb.MemberUpdate_Left_{
				Left: &chatpb.MemberUpdate_Left{
					Member: userID,
				},
			},
		},
	}})
	if err != nil {
		s.log.Warn("Failed to notify member leaving", zap.Error(err))
	}

	return &chatpb.LeaveChatResponse{}, nil
}

func (s *Server) OpenChat(ctx context.Context, req *chatpb.OpenChatRequest) (*chatpb.OpenChatResponse, error) {
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
		return &chatpb.OpenChatResponse{Result: chatpb.OpenChatResponse_DENIED}, nil
	} else if err != nil {
		log.Warn("Failed to get chat", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat")
	}

	if md.Type != chatpb.Metadata_GROUP {
		return &chatpb.OpenChatResponse{Result: chatpb.OpenChatResponse_DENIED}, nil
	}
	if md.Owner == nil || !bytes.Equal(md.Owner.Value, userID.Value) {
		return &chatpb.OpenChatResponse{Result: chatpb.OpenChatResponse_DENIED}, nil
	}

	if md.OpenStatus.IsCurrentlyOpen {
		return &chatpb.OpenChatResponse{}, nil
	}

	err = s.chats.SetOpenStatus(ctx, req.ChatId, true)
	if err != nil {
		s.log.Warn("Failed to open chat", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to open chat")
	}

	err = s.eventBus.OnEvent(req.ChatId, &event.ChatEvent{ChatID: md.ChatId, MetadataUpdates: []*chatpb.MetadataUpdate{
		{
			Kind: &chatpb.MetadataUpdate_OpenStatusChanged_{
				OpenStatusChanged: &chatpb.MetadataUpdate_OpenStatusChanged{
					NewOpenStatus: &chatpb.OpenStatus{
						IsCurrentlyOpen: true,
					},
				},
			},
		},
	}})
	if err != nil {
		s.log.Warn("Failed to notify chat is open", zap.Error(err))
	}

	return &chatpb.OpenChatResponse{}, nil
}

func (s *Server) CloseChat(ctx context.Context, req *chatpb.CloseChatRequest) (*chatpb.CloseChatResponse, error) {
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
		return &chatpb.CloseChatResponse{Result: chatpb.CloseChatResponse_DENIED}, nil
	} else if err != nil {
		log.Warn("Failed to get chat", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat")
	}

	if md.Type != chatpb.Metadata_GROUP {
		return &chatpb.CloseChatResponse{Result: chatpb.CloseChatResponse_DENIED}, nil
	}
	if md.Owner == nil || !bytes.Equal(md.Owner.Value, userID.Value) {
		return &chatpb.CloseChatResponse{Result: chatpb.CloseChatResponse_DENIED}, nil
	}

	if !md.OpenStatus.IsCurrentlyOpen {
		return &chatpb.CloseChatResponse{}, nil
	}

	err = s.chats.SetOpenStatus(ctx, req.ChatId, false)
	if err != nil {
		s.log.Warn("Failed to close chat", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to close chat")
	}

	err = s.eventBus.OnEvent(req.ChatId, &event.ChatEvent{ChatID: md.ChatId, MetadataUpdates: []*chatpb.MetadataUpdate{
		{
			Kind: &chatpb.MetadataUpdate_OpenStatusChanged_{
				OpenStatusChanged: &chatpb.MetadataUpdate_OpenStatusChanged{
					NewOpenStatus: &chatpb.OpenStatus{
						IsCurrentlyOpen: false,
					},
				},
			},
		},
	}})
	if err != nil {
		s.log.Warn("Failed to notify chat is closed", zap.Error(err))
	}

	return &chatpb.CloseChatResponse{}, nil
}

// todo: this RPC needs tests
func (s *Server) CheckDisplayName(ctx context.Context, req *chatpb.CheckDisplayNameRequest) (*chatpb.CheckDisplayNameResponse, error) {
	log := s.log.With(zap.String("display_name", req.DisplayName))

	moderationResult, err := s.moderationClient.ClassifyText(req.DisplayName)
	if err != nil {
		log.Warn("Failed to moderate display name", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to moderate display name")
	}

	return &chatpb.CheckDisplayNameResponse{
		Result:    chatpb.CheckDisplayNameResponse_OK,
		IsAllowed: !moderationResult.Flagged,
	}, nil
}

func (s *Server) SetDisplayName(ctx context.Context, req *chatpb.SetDisplayNameRequest) (*chatpb.SetDisplayNameResponse, error) {
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
		return &chatpb.SetDisplayNameResponse{Result: chatpb.SetDisplayNameResponse_DENIED}, nil
	} else if err != nil {
		log.Warn("Failed to get chat", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat")
	}

	if md.Type != chatpb.Metadata_GROUP {
		return &chatpb.SetDisplayNameResponse{Result: chatpb.SetDisplayNameResponse_DENIED}, nil
	}
	if md.Owner == nil || !bytes.Equal(md.Owner.Value, userID.Value) {
		return &chatpb.SetDisplayNameResponse{Result: chatpb.SetDisplayNameResponse_DENIED}, nil
	}

	if md.DisplayName == req.DisplayName {
		return &chatpb.SetDisplayNameResponse{}, nil
	}

	// todo: this needs tests
	if len(req.DisplayName) > 0 {
		log = log.With(zap.String("display_name", req.DisplayName))

		moderationResult, err := s.moderationClient.ClassifyText(req.DisplayName)
		if err != nil {
			log.Warn("Failed to moderate display name", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to moderate display name")
		} else if moderationResult.Flagged {
			return &chatpb.SetDisplayNameResponse{Result: chatpb.SetDisplayNameResponse_CANT_SET}, nil
		}
	}

	err = s.chats.SetDisplayName(ctx, req.ChatId, req.DisplayName)
	if err != nil {
		log.Warn("Failed to set display name", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to set display name")
	}

	go func() {
		ctx := context.Background()

		var announcementContentBuilder messaging.AnnouncementContentBuilder
		if len(req.DisplayName) > 0 {
			announcementContentBuilder = messaging.NewRoomDisplayNameChangedAnnouncementContentBuilder(md.RoomNumber, req.DisplayName)
		} else {
			announcementContentBuilder = messaging.NewRoomDisplayNameRemovedAnnouncementContentBuilder()
		}
		if err = messaging.SendAnnouncement(
			ctx,
			s.messenger,
			req.ChatId,
			announcementContentBuilder,
		); err != nil {
			log.Warn("Failed to send announcement", zap.Error(err))
		}

		err = s.eventBus.OnEvent(req.ChatId, &event.ChatEvent{ChatID: md.ChatId, MetadataUpdates: []*chatpb.MetadataUpdate{
			{
				Kind: &chatpb.MetadataUpdate_DisplayNameChanged_{
					DisplayNameChanged: &chatpb.MetadataUpdate_DisplayNameChanged{
						NewDisplayName: req.DisplayName,
					},
				},
			},
		}})
		if err != nil {
			s.log.Warn("Failed to notify display name changed", zap.Error(err))
		}
	}()

	return &chatpb.SetDisplayNameResponse{}, nil
}

// todo: Deprecate this RPC
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

	if md.Type != chatpb.Metadata_GROUP {
		return &chatpb.SetCoverChargeResponse{Result: chatpb.SetCoverChargeResponse_DENIED}, nil
	}
	if md.Owner == nil || !bytes.Equal(md.Owner.Value, userID.Value) {
		return &chatpb.SetCoverChargeResponse{Result: chatpb.SetCoverChargeResponse_DENIED}, nil
	}
	if md.MessagingFee == nil {
		return &chatpb.SetCoverChargeResponse{Result: chatpb.SetCoverChargeResponse_CANT_SET}, nil
	}

	if md.MessagingFee.Quarks == req.CoverCharge.Quarks {
		return &chatpb.SetCoverChargeResponse{}, nil
	}

	err = s.chats.SetMessagingFee(ctx, req.ChatId, req.CoverCharge)
	if err != nil {
		log.Warn("Failed to set cover charge", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to set cover charge")
	}

	go func() {
		ctx := context.Background()

		if err = messaging.SendAnnouncement(
			ctx,
			s.messenger,
			req.ChatId,
			messaging.NewCoverChangedAnnouncementContentBuilder(req.CoverCharge.Quarks),
		); err != nil {
			log.Warn("Failed to send announcement", zap.Error(err))
		}

		err = s.eventBus.OnEvent(req.ChatId, &event.ChatEvent{ChatID: md.ChatId, MetadataUpdates: []*chatpb.MetadataUpdate{
			{
				Kind: &chatpb.MetadataUpdate_MessagingFeeChanged_{
					MessagingFeeChanged: &chatpb.MetadataUpdate_MessagingFeeChanged{
						NewMessagingFee: req.CoverCharge,
					},
				},
			},
		}})
		if err != nil {
			s.log.Warn("Failed to notify cover changed", zap.Error(err))
		}
	}()

	return &chatpb.SetCoverChargeResponse{}, nil
}

func (s *Server) SetMessagingFee(ctx context.Context, req *chatpb.SetMessagingFeeRequest) (*chatpb.SetMessagingFeeResponse, error) {
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
		return &chatpb.SetMessagingFeeResponse{Result: chatpb.SetMessagingFeeResponse_DENIED}, nil
	} else if err != nil {
		log.Warn("Failed to get chat", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat")
	}

	if md.Type != chatpb.Metadata_GROUP {
		return &chatpb.SetMessagingFeeResponse{Result: chatpb.SetMessagingFeeResponse_DENIED}, nil
	}
	if md.Owner == nil || !bytes.Equal(md.Owner.Value, userID.Value) {
		return &chatpb.SetMessagingFeeResponse{Result: chatpb.SetMessagingFeeResponse_DENIED}, nil
	}
	if md.MessagingFee == nil {
		return &chatpb.SetMessagingFeeResponse{Result: chatpb.SetMessagingFeeResponse_CANT_SET}, nil
	}

	if md.MessagingFee.Quarks == req.MessagingFee.Quarks {
		return &chatpb.SetMessagingFeeResponse{}, nil
	}

	err = s.chats.SetMessagingFee(ctx, req.ChatId, req.MessagingFee)
	if err != nil {
		log.Warn("Failed to set cover charge", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to set messaging fee")
	}

	go func() {
		ctx := context.Background()

		if err = messaging.SendAnnouncement(
			ctx,
			s.messenger,
			req.ChatId,
			messaging.NewMessagingFeeChangedAnnouncementContentBuilder(req.MessagingFee.Quarks),
		); err != nil {
			log.Warn("Failed to send announcement", zap.Error(err))
		}

		err = s.eventBus.OnEvent(req.ChatId, &event.ChatEvent{ChatID: md.ChatId, MetadataUpdates: []*chatpb.MetadataUpdate{
			{
				Kind: &chatpb.MetadataUpdate_MessagingFeeChanged_{
					MessagingFeeChanged: &chatpb.MetadataUpdate_MessagingFeeChanged{
						NewMessagingFee: req.MessagingFee,
					},
				},
			},
		}})
		if err != nil {
			s.log.Warn("Failed to notify messaging fee changed", zap.Error(err))
		}
	}()

	return &chatpb.SetMessagingFeeResponse{}, nil
}

// todo: use a proper changelog system, but for now use a full refresh
func (s *Server) GetMemberUpdates(ctx context.Context, req *chatpb.GetMemberUpdatesRequest) (*chatpb.GetMemberUpdatesResponse, error) {
	userID, err := s.authz.Authorize(ctx, req, &req.Auth)
	if err != nil {
		return nil, err
	}

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("chat_id", base64.StdEncoding.EncodeToString(req.ChatId.Value)),
	)

	if req.PagingToken != nil {
		if len(req.PagingToken.Value) != 8 {
			return nil, status.Error(codes.InvalidArgument, "paging token length must be 8")
		}

		// Will result in eventual consistency issues between client and server
		unixTs := binary.LittleEndian.Uint64(req.PagingToken.Value)
		if time.Since(time.Unix(int64(unixTs), 0)) < time.Minute {
			return &chatpb.GetMemberUpdatesResponse{Result: chatpb.GetMemberUpdatesResponse_NOT_FOUND}, nil
		}
	}

	_, members, err := s.getMetadataWithMembers(ctx, req.ChatId, userID)
	if err == ErrChatNotFound {
		return &chatpb.GetMemberUpdatesResponse{Result: chatpb.GetMemberUpdatesResponse_NOT_FOUND}, nil
	} else if err != nil {
		log.Warn("Failed to get chat members", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "failed to get chat members")
	}

	pagingToken := make([]byte, 8)
	binary.LittleEndian.PutUint64(pagingToken, uint64(time.Now().Unix()))

	return &chatpb.GetMemberUpdatesResponse{Updates: []*chatpb.MemberUpdate{
		{
			Kind: &chatpb.MemberUpdate_FullRefresh_{
				FullRefresh: &chatpb.MemberUpdate_FullRefresh{
					Members: members,
				},
			},
			PagingToken: &commonpb.PagingToken{
				Value: pagingToken,
			},
		},
	}}, nil
}

func (s *Server) PromoteUser(ctx context.Context, req *chatpb.PromoteUserRequest) (*chatpb.PromoteUserResponse, error) {
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
		return &chatpb.PromoteUserResponse{Result: chatpb.PromoteUserResponse_DENIED}, nil
	}

	md, err := s.getMetadata(ctx, req.ChatId, nil)
	if err == ErrChatNotFound {
		return &chatpb.PromoteUserResponse{Result: chatpb.PromoteUserResponse_DENIED}, nil
	} else if err != nil {
		log.Warn("Failed to get chat data", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat data")
	}

	if md.Type != chatpb.Metadata_GROUP {
		return &chatpb.PromoteUserResponse{Result: chatpb.PromoteUserResponse_DENIED}, nil
	}

	if md.Owner == nil || !bytes.Equal(md.Owner.Value, ownerID.Value) {
		return &chatpb.PromoteUserResponse{Result: chatpb.PromoteUserResponse_DENIED}, nil
	}

	memberToPromote, err := s.chats.GetMember(ctx, req.ChatId, req.UserId)
	if err == ErrMemberNotFound {
		return &chatpb.PromoteUserResponse{Result: chatpb.PromoteUserResponse_DENIED}, nil
	} else if err != nil {
		log.Warn("Failed to get chat member", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat member")
	}

	if req.EnableSendPermission {
		if memberToPromote.HasSendPermission {
			return &chatpb.PromoteUserResponse{}, nil
		}

		err := s.chats.SetSendPermission(ctx, req.ChatId, req.UserId, true)
		if err != nil {
			log.Warn("Failed to set send permission", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to set send permission")
		}

		go func() {
			ctx := context.Background()

			if err = messaging.SendAnnouncement(
				ctx,
				s.messenger,
				req.ChatId,
				messaging.NewUserPromotedToSpeakerAnnouncementContentBuilder(ctx, s.profiles, ownerID, req.UserId),
			); err != nil {
				log.Warn("Failed to send announcement", zap.Error(err))
			}

			err = s.eventBus.OnEvent(req.ChatId, &event.ChatEvent{ChatID: req.ChatId, MemberUpdates: []*chatpb.MemberUpdate{
				{
					Kind: &chatpb.MemberUpdate_Promoted_{
						Promoted: &chatpb.MemberUpdate_Promoted{
							Member:                req.UserId,
							PromotedBy:            ownerID,
							SendPermissionEnabled: true,
						},
					},
				},
			}})
			if err != nil {
				s.log.Warn("Failed to notify member demoted", zap.Error(err))
			}
		}()
	}

	return &chatpb.PromoteUserResponse{}, nil
}

func (s *Server) DemoteUser(ctx context.Context, req *chatpb.DemoteUserRequest) (*chatpb.DemoteUserResponse, error) {
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
		return &chatpb.DemoteUserResponse{Result: chatpb.DemoteUserResponse_DENIED}, nil
	}

	md, err := s.getMetadata(ctx, req.ChatId, nil)
	if err == ErrChatNotFound {
		return &chatpb.DemoteUserResponse{Result: chatpb.DemoteUserResponse_DENIED}, nil
	} else if err != nil {
		log.Warn("Failed to get chat data", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat data")
	}

	if md.Type != chatpb.Metadata_GROUP {
		return &chatpb.DemoteUserResponse{Result: chatpb.DemoteUserResponse_DENIED}, nil
	}

	if md.Owner == nil || !bytes.Equal(md.Owner.Value, ownerID.Value) {
		return &chatpb.DemoteUserResponse{Result: chatpb.DemoteUserResponse_DENIED}, nil
	}

	memberToDemote, err := s.chats.GetMember(ctx, req.ChatId, req.UserId)
	if err == ErrMemberNotFound {
		return &chatpb.DemoteUserResponse{Result: chatpb.DemoteUserResponse_DENIED}, nil
	} else if err != nil {
		log.Warn("Failed to get chat member", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat member")
	}

	if req.DisableSendPermission {
		if !memberToDemote.HasSendPermission {
			return &chatpb.DemoteUserResponse{}, nil
		}

		err := s.chats.SetSendPermission(ctx, req.ChatId, req.UserId, false)
		if err != nil {
			log.Warn("Failed to set send permission", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to set send permission")
		}

		go func() {
			// todo: announcement?

			err = s.eventBus.OnEvent(req.ChatId, &event.ChatEvent{ChatID: req.ChatId, MemberUpdates: []*chatpb.MemberUpdate{
				{
					Kind: &chatpb.MemberUpdate_Demoted_{
						Demoted: &chatpb.MemberUpdate_Demoted{
							Member:                 req.UserId,
							DemotedBy:              ownerID,
							SendPermissionDisabled: true,
						},
					},
				},
			}})
			if err != nil {
				s.log.Warn("Failed to notify member demoted", zap.Error(err))
			}
		}()
	}

	return &chatpb.DemoteUserResponse{}, nil
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

		isMember, err := s.chats.IsMember(ctx, req.ChatId, req.UserId)
		if err != nil {
			log.Warn("Failed to get chat membership status", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to get chat membership status")
		}
		if !isMember {
			return &chatpb.RemoveUserResponse{}, nil
		}

		if err = s.chats.RemoveMember(ctx, req.ChatId, req.UserId); err != nil {
			log.Warn("Failed to remove member", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "failed to remove chat member")
		}

		go func() {
			ctx := context.Background()

			if err = messaging.SendAnnouncement(
				ctx,
				s.messenger,
				req.ChatId,
				messaging.NewUserRemovedAnnouncementContentBuilder(ctx, s.profiles, req.UserId),
			); err != nil {
				log.Warn("Failed to send announcement", zap.Error(err))
			}

			err = s.eventBus.OnEvent(req.ChatId, &event.ChatEvent{ChatID: md.ChatId, MemberUpdates: []*chatpb.MemberUpdate{
				{
					Kind: &chatpb.MemberUpdate_Removed_{
						Removed: &chatpb.MemberUpdate_Removed{
							Member:    req.UserId,
							RemovedBy: ownerID,
						},
					},
				},
			}})
			if err != nil {
				s.log.Warn("Failed to notify removed member", zap.Error(err))
			}
		}()

		return &chatpb.RemoveUserResponse{}, nil
	*/
}

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

	memberToMute, err := s.chats.GetMember(ctx, req.ChatId, req.UserId)
	if err == ErrMemberNotFound {
		return &chatpb.MuteUserResponse{Result: chatpb.MuteUserResponse_DENIED}, nil
	} else if err != nil {
		log.Warn("Failed to get chat member", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get chat member")
	}
	if memberToMute.IsMuted {
		return &chatpb.MuteUserResponse{}, nil
	}

	if err = s.chats.SetMuteState(ctx, req.ChatId, req.UserId, true); err != nil {
		log.Warn("Failed to mute chat member", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to mute chat member")
	}

	go func() {
		ctx := context.Background()

		if err = messaging.SendAnnouncement(
			ctx,
			s.messenger,
			req.ChatId,
			messaging.NewUserMutedAnnouncementContentBuilder(ctx, s.profiles, ownerID, req.UserId),
		); err != nil {
			log.Warn("Failed to send announcement", zap.Error(err))
		}

		err = s.eventBus.OnEvent(req.ChatId, &event.ChatEvent{ChatID: req.ChatId, MemberUpdates: []*chatpb.MemberUpdate{
			{
				Kind: &chatpb.MemberUpdate_Muted_{
					Muted: &chatpb.MemberUpdate_Muted{
						Member:  req.UserId,
						MutedBy: ownerID,
					},
				},
			},
		}})
		if err != nil {
			s.log.Warn("Failed to notify muted member", zap.Error(err))
		}
	}()

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
		log.Warn("Failed to unmute chat", zap.Error(err))
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

	// Every new chat message with user content advances the last activity timestamp
	// and is injected as part of the event
	if clonedEvent.MessageUpdate != nil {
		switch clonedEvent.MessageUpdate.Content[0].Type.(type) {
		case *messagingpb.Content_Text, *messagingpb.Content_Reply:
			err := s.chats.AdvanceLastChatActivity(context.Background(), chatID, clonedEvent.MessageUpdate.Ts.AsTime())
			if err != nil {
				s.log.Warn("Failed to advance chat activity timestamp", zap.Error(err))
			}

			clonedEvent.MetadataUpdates = append(clonedEvent.MetadataUpdates, &chatpb.MetadataUpdate{
				Kind: &chatpb.MetadataUpdate_LastActivityChanged_{
					LastActivityChanged: &chatpb.MetadataUpdate_LastActivityChanged{
						NewLastActivity: clonedEvent.MessageUpdate.Ts,
					},
				},
			})
		}
	}

	members, err := s.chats.GetMembers(context.Background(), chatID)
	if err != nil {
		s.log.Warn("Failed to get chat members for notification", zap.Error(err))
		return
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

// todo: duplicated code with getMetadataBatched
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

	numUnread, hasMoreUnread, err := s.getUnreadCount(ctx, chatID, caller, nil)
	if err != nil {
		return nil, err
	}
	md.NumUnread = numUnread
	md.HasMoreUnread = hasMoreUnread

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

// todo: duplicated code with getMetadata
func (s *Server) getMetadataBatched(ctx context.Context, chatIDs []*commonpb.ChatId, caller *commonpb.UserId) ([]*chatpb.Metadata, error) {
	metadata, err := s.chats.GetChatMetadataBatched(ctx, chatIDs...)
	if err != nil {
		return nil, err
	}

	for _, md := range metadata {
		md.CanDisablePush = true
	}

	// If the caller is not specified, _or_ the caller isn't a member, we don't need to fill out
	// caller specific fields.
	if caller == nil {
		return metadata, nil
	}

	for _, md := range metadata {
		numUnread, hasMoreUnread, err := s.getUnreadCount(ctx, md.ChatId, caller, nil)
		if err != nil {
			return nil, err
		}
		md.NumUnread = numUnread
		md.HasMoreUnread = hasMoreUnread

		// todo: this needs testing
		isPushEnabled, err := s.chats.IsPushEnabled(ctx, md.ChatId, caller)
		if err == ErrMemberNotFound {
			isPushEnabled = true
		} else if err != nil {
			return nil, fmt.Errorf("failed to get push state: %w", err)
		}
		md.IsPushEnabled = isPushEnabled
	}

	return metadata, nil
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
			p.HasModeratorPermission = true
			p.HasSendPermission = true
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
		m.Identity = &chatpb.MemberIdentity{
			DisplayName: p.GetDisplayName(),
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

func (s *Server) getUnreadCount(ctx context.Context, chatID *commonpb.ChatId, caller *commonpb.UserId, readPtr *messagingpb.MessageId) (uint32, bool, error) {
	if readPtr == nil {
		ptrs, err := s.pointers.GetPointers(ctx, chatID, caller)
		if err != nil {
			return 0, false, fmt.Errorf("failed to get caller pointers: %w", err)
		}

		for _, ptr := range ptrs {
			if ptr.Type == messagingpb.Pointer_READ {
				readPtr = ptr.Value
				break
			}
		}
	}

	unread, err := s.messages.CountUnread(ctx, chatID, caller, readPtr, int64(MaxUnreadCount+1))
	if err != nil {
		return 0, false, fmt.Errorf("failed to count unread messages: %w", err)
	}

	if uint32(unread) > MaxUnreadCount {
		return MaxUnreadCount, true, nil
	}
	return uint32(unread), false, nil
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

	metadata, err := s.getMetadataBatched(ctx, chatIDs, userID)
	if err != nil {
		log.Warn("Failed to get metadata for chats (stream flush)", zap.Error(err))
		return
	}

	events := make([]*event.ChatEvent, len(metadata))
	for i, md := range metadata {
		log := log.With(zap.String("user_id", base64.StdEncoding.EncodeToString(md.ChatId.Value)))

		e := &event.ChatEvent{
			ChatID: md.ChatId,
			MetadataUpdates: []*chatpb.MetadataUpdate{
				{
					Kind: &chatpb.MetadataUpdate_FullRefresh_{
						FullRefresh: &chatpb.MetadataUpdate_FullRefresh{
							Metadata: md,
						},
					},
				},
			},
		}
		events[i] = e

		messages, err := s.messages.GetPagedMessages(ctx, e.ChatID, query.WithDescending(), query.WithLimit(1))
		if err != nil {
			log.Warn("Failed to get last message for chat (stream flush)", zap.Error(err))
		} else if len(messages) > 0 {
			e.MessageUpdate = messages[len(messages)-1]
		}
	}

	sort.Slice(events, func(i, j int) bool {
		timestampAtI := events[i].MetadataUpdates[0].GetFullRefresh().Metadata.LastActivity.AsTime()
		timestampAtJ := events[j].MetadataUpdates[0].GetFullRefresh().Metadata.LastActivity.AsTime()
		return timestampAtI.After(timestampAtJ)
	})

	var batch []*event.ChatEvent
	for _, e := range events {
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
