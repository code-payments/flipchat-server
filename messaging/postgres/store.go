package postgres

import (
	"bytes"
	"context"
	"errors"
	"slices"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
	"google.golang.org/protobuf/proto"

	pg "github.com/code-payments/flipchat-server/database/postgres"
	"github.com/code-payments/flipchat-server/database/prisma/db"
	"github.com/code-payments/flipchat-server/messaging"
	"github.com/code-payments/flipchat-server/query"
)

type store struct {
	client *db.PrismaClient
}

func NewInPostgresMessages(client *db.PrismaClient) messaging.MessageStore {
	return &store{
		client,
	}
}

func NewInPostgresPointers(client *db.PrismaClient) messaging.PointerStore {
	return &store{
		client,
	}
}

func (s *store) reset() {
	ctx := context.Background()

	pointers := s.client.Pointer.FindMany().Delete().Tx()
	messages := s.client.Message.FindMany().Delete().Tx()

	err := s.client.Prisma.Transaction(pointers, messages).Exec(ctx)
	if err != nil {
		panic(err)
	}
}

func (s *store) GetMessages(ctx context.Context, chatID *commonpb.ChatId, options ...query.Option) ([]*messagingpb.Message, error) {
	encodedChatID := pg.Encode(chatID.Value)

	messages, err := s.client.Message.FindMany(
		db.Message.ChatID.Equals(encodedChatID),
	).Exec(ctx)

	if err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, nil
	}

	result := make([]*messagingpb.Message, len(messages))
	for i, message := range messages {

		pgMessage := &messagingpb.Message{}

		err := proto.Unmarshal(message.Content, pgMessage)
		if err != nil {
			return nil, err
		}

		result[i] = pgMessage
	}

	// TODO: Add pagination
	// TODO: Consider sorting in the database query.

	// We're sorting here because the in-memory version uses a byte level sort
	// on the message id. The database can't do that. Consider changing the
	// in-memory version to use a time based sort instead. Or use an
	// auto-incrementing id.

	slices.SortFunc(result, func(a, b *messagingpb.Message) int {
		return bytes.Compare(a.MessageId.Value, b.MessageId.Value)
	})

	return result, nil
}

func (s *store) PutMessage(ctx context.Context, chatID *commonpb.ChatId, msg *messagingpb.Message) error {
	if msg.MessageId != nil {
		return errors.New("cannt provide a message id")
	}

	msg.MessageId = messaging.MustGenerateMessageID()

	encodedChatID := pg.Encode(chatID.Value)
	encodedMessageId := pg.Encode(msg.MessageId.Value)
	encodedSenderId := pg.Encode(msg.SenderId.Value)

	// Note, we're storing the whole message as a serialized blob because we
	// can't serialze just the content.

	// TODO: consider adding a wrapper around the Content[] field to make it
	// possible to serialize just the content.

	serializedMessage, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = s.client.Message.CreateOne(
		db.Message.ID.Set(encodedMessageId),
		db.Message.ChatID.Set(encodedChatID),
		db.Message.SenderID.Set(encodedSenderId),
		db.Message.Content.Set(serializedMessage),
	).Exec(ctx)

	return err
}

func (s *store) CountUnread(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, lastRead *messagingpb.MessageId) (int64, error) {
	encodedChatID := pg.Encode(chatID.Value)
	encodedUserID := pg.Encode(userID.Value)

	// Query arguments
	queryArgs := []db.MessageWhereParam{
		db.Message.ChatID.Equals(encodedChatID),
		db.Message.SenderID.Not(encodedUserID),
	}

	// Conditionally add the createdAt condition if lastRead is provided
	if lastRead != nil {
		encodedMessageId := pg.Encode(lastRead.Value)
		msg, err := s.client.Message.FindUnique(
			db.Message.ID.Equals(encodedMessageId),
		).Exec(ctx)
		if err != nil {
			return 0, err
		}
		queryArgs = append(queryArgs, db.Message.CreatedAt.Gt(msg.CreatedAt))
	}

	// Perform the query

	// TODO: prisma-client-go doesn't support Count() yet, convert this to a raw
	// query at some point. For now, we'll just fetch all the messages and count
	// them. Using a Select() to reduce the amount of data fetched.

	messages, err := s.client.Message.FindMany(
		queryArgs...,
	).OrderBy(
		db.Message.CreatedAt.Order(db.SortOrderAsc),
	).Select(
		db.Chat.ID.Field(),
	).Exec(ctx)

	if err != nil {
		return 0, err
	}

	return int64(len(messages)), nil
}

func (s *store) AdvancePointer(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, pointer *messagingpb.Pointer) (bool, error) {
	/*
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
	*/

	return true, nil
}

func (s *store) GetPointers(_ context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) ([]*messagingpb.Pointer, error) {
	/*
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
	*/

	return nil, nil
}

func (s *store) GetAllPointers(_ context.Context, chatID *commonpb.ChatId) ([]messaging.UserPointer, error) {
	/*
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
	*/

	return nil, nil
}
