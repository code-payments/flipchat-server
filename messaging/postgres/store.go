package postgres

import (
	"bytes"
	"context"
	"errors"

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

	appliedOptions := query.ApplyOptions(options...)

	findMany := s.client.Message.FindMany(
		db.Message.ChatID.Equals(encodedChatID),
	)
	if appliedOptions.Token != nil {
		findMany = findMany.Cursor(
			db.Message.ID.Cursor(appliedOptions.Token.Value),
		)
	}
	messages, err := findMany.OrderBy(
		db.Message.ID.Order(query.ToPrismaSortOrder(appliedOptions.Order)),
	).Take(
		appliedOptions.Limit + 1, // Because cursor returns the row associated with the ID
	).Exec(ctx)

	if err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, nil
	}

	result := make([]*messagingpb.Message, 0)
	for _, message := range messages {
		if appliedOptions.Token != nil && bytes.Equal(appliedOptions.Token.Value, message.ID) {
			continue
		}

		protoMessage := &messagingpb.Message{}

		err := proto.Unmarshal(message.Content, protoMessage)
		if err != nil {
			return nil, err
		}

		result = append(result, protoMessage)
	}

	if len(result) > appliedOptions.Limit {
		result = result[:appliedOptions.Limit]
	}
	return result, nil
}

func (s *store) PutMessage(ctx context.Context, chatID *commonpb.ChatId, msg *messagingpb.Message) error {
	if msg.MessageId != nil {
		return errors.New("cannt provide a message id")
	}

	msg.MessageId = messaging.MustGenerateMessageID()

	encodedChatID := pg.Encode(chatID.Value)

	// Note, we're storing the whole message as a serialized blob because we
	// can't serialze just the content.

	// TODO: consider adding a wrapper around the Content[] field to make it
	// possible to serialize just the content.

	serializedMessage, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	opt := []db.MessageSetParam{}
	if msg.SenderId != nil {
		encodedSenderId := pg.Encode(msg.SenderId.Value)
		opt = append(opt, db.Message.SenderID.Set(encodedSenderId))
	}

	_, err = s.client.Message.CreateOne(
		db.Message.ID.Set(msg.MessageId.Value),
		db.Message.ChatID.Set(encodedChatID),
		db.Message.Content.Set(serializedMessage),
		opt...,
	).Exec(ctx)

	return err
}

func (s *store) CountUnread(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, lastRead *messagingpb.MessageId, maxValue int64) (int64, error) {
	encodedChatID := pg.Encode(chatID.Value)
	encodedUserID := pg.Encode(userID.Value)

	// Query arguments
	queryArgs := []db.MessageWhereParam{
		db.Message.ChatID.Equals(encodedChatID),
		db.Message.Or(
			db.Message.SenderID.IsNull(),
			db.Message.Not(db.Message.SenderID.Equals(encodedUserID)),
		),
	}

	if lastRead != nil {
		queryArgs = append(queryArgs, db.Message.ID.Not(lastRead.Value))
	}

	// Perform the query

	// TODO: prisma-client-go doesn't support Count() yet, convert this to a raw
	// query at some point. For now, we'll just fetch all the messages and count
	// them. Using a Select() to reduce the amount of data fetched.

	findMany := s.client.Message.FindMany(
		queryArgs...,
	).Select(
		db.Chat.ID.Field(),
	)

	if lastRead != nil {
		findMany = findMany.Cursor(db.Message.ID.Cursor(lastRead.Value))
	}

	if maxValue >= 0 {
		findMany = findMany.Take(int(maxValue))
	}

	messages, err := findMany.Exec(ctx)

	if err != nil {
		return 0, err
	}

	return int64(len(messages)), nil
}

func (s *store) AdvancePointer(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, pointer *messagingpb.Pointer) (bool, error) {

	encodedChatID := pg.Encode(chatID.Value)
	encodedUserID := pg.Encode(userID.Value)
	encodedMessageId := pg.Encode(pointer.Value.Value)

	// Find the existing pointer (if any)
	existingPointer, err := s.client.Pointer.FindUnique(
		db.Pointer.ChatIDUserIDType(
			db.Pointer.ChatID.Equals(encodedChatID),
			db.Pointer.UserID.Equals(encodedUserID),
			db.Pointer.Type.Equals(int(pointer.Type)),
		),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) || existingPointer == nil {
		// Pointer doesn't exist, create it

		_, err := s.client.Pointer.CreateOne(
			db.Pointer.ChatID.Set(encodedChatID),
			db.Pointer.UserID.Set(encodedUserID),
			db.Pointer.Value.Set(encodedMessageId),
			db.Pointer.Type.Set(int(pointer.Type)),
		).Exec(ctx)

		return true, err
	} else if err != nil {
		return false, err
	}

	decodedExistingMessageId, err := pg.Decode(existingPointer.Value)
	if err != nil {
		return false, err
	}

	// If the existing pointer is already ahead of the new pointer, don't update
	if bytes.Compare(decodedExistingMessageId, pointer.Value.Value) >= 0 {
		return false, nil
	}

	// Update the pointer with the new value
	s.client.Pointer.FindUnique(
		db.Pointer.ChatIDUserIDType(
			db.Pointer.ChatID.Equals(encodedChatID),
			db.Pointer.UserID.Equals(encodedUserID),
			db.Pointer.Type.Equals(int(pointer.Type)),
		),
	).Update(
		db.Pointer.Value.Set(encodedMessageId),
	).Exec(ctx)

	return true, nil
}

func (s *store) GetPointers(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) ([]*messagingpb.Pointer, error) {

	encodedChatID := pg.Encode(chatID.Value)
	encodedUserID := pg.Encode(userID.Value)

	pointers, err := s.client.Pointer.FindMany(
		db.Pointer.ChatID.Equals(encodedChatID),
		db.Pointer.UserID.Equals(encodedUserID),
	).Exec(ctx)

	if err != nil {
		return nil, err
	}

	pgPointers := make([]*messagingpb.Pointer, len(pointers))

	for i, pointer := range pointers {
		decodedMessageId, err := pg.Decode(pointer.Value)
		if err != nil {
			return nil, err
		}

		pgPointers[i] = &messagingpb.Pointer{
			Type:  messagingpb.Pointer_Type(pointer.Type),
			Value: &messagingpb.MessageId{Value: decodedMessageId},
		}
	}

	return pgPointers, nil
}

func (s *store) GetAllPointers(ctx context.Context, chatID *commonpb.ChatId) ([]messaging.UserPointer, error) {

	encodedChatID := pg.Encode(chatID.Value)

	pointers, err := s.client.Pointer.FindMany(
		db.Pointer.ChatID.Equals(encodedChatID),
	).Exec(ctx)

	if err != nil {
		return nil, err
	}

	pgPointers := make([]messaging.UserPointer, len(pointers))
	for i, pointer := range pointers {

		decodedUserId, err := pg.Decode(pointer.UserID)
		if err != nil {
			return nil, err
		}

		decodedMessageId, err := pg.Decode(pointer.Value)
		if err != nil {
			return nil, err
		}

		pgPointers[i] = messaging.UserPointer{
			UserID: &commonpb.UserId{Value: decodedUserId},
			Pointer: &messagingpb.Pointer{
				Type:  messagingpb.Pointer_Type(pointer.Type),
				Value: &messagingpb.MessageId{Value: decodedMessageId},
			},
		}
	}

	return pgPointers, nil
}
