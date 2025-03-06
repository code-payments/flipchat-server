package postgres

import (
	"bytes"
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	pg "github.com/code-payments/flipchat-server/database/postgres"
	"github.com/code-payments/flipchat-server/messaging"
	"github.com/code-payments/flipchat-server/query"
)

type store struct {
	pool *pgxpool.Pool
}

func NewInPostgresMessages(pool *pgxpool.Pool) messaging.MessageStore {
	return &store{
		pool: pool,
	}
}

func NewInPostgresPointers(pool *pgxpool.Pool) messaging.PointerStore {
	return &store{
		pool: pool,
	}
}

func (s *store) GetMessage(ctx context.Context, chatID *commonpb.ChatId, messageID *messagingpb.MessageId) (*messagingpb.Message, error) {
	model, err := dbGetMessage(ctx, s.pool, chatID, messageID)
	if err != nil {
		return nil, err
	}
	return fromMessageModel(model)
}

func (s *store) GetBatchMessages(ctx context.Context, chatID *commonpb.ChatId, messageIDs ...*messagingpb.MessageId) ([]*messagingpb.Message, error) {
	models, err := dbGetBatchMessages(ctx, s.pool, chatID, messageIDs...)
	if err != nil {
		return nil, err
	}

	res := make([]*messagingpb.Message, len(models))
	for i, model := range models {
		res[i], err = fromMessageModel(model)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (s *store) GetPagedMessages(ctx context.Context, chatID *commonpb.ChatId, options ...query.Option) ([]*messagingpb.Message, error) {
	models, err := dbGetPagedMessages(ctx, s.pool, chatID, options...)
	if err != nil {
		return nil, err
	}

	res := make([]*messagingpb.Message, len(models))
	for i, model := range models {
		res[i], err = fromMessageModel(model)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (s *store) PutMessage(ctx context.Context, chatID *commonpb.ChatId, msg *messagingpb.Message) (*messagingpb.Message, error) {
	if msg.MessageId != nil {
		return nil, errors.New("cannot provide a message id")
	}

	msg = proto.Clone(msg).(*messagingpb.Message)
	msg.MessageId = messaging.MustGenerateMessageID()

	model, err := toContentMessageModel(chatID, msg)
	if err != nil {
		return nil, err
	}

	err = model.dbPut(ctx, s.pool)
	if err != nil {
		return nil, err
	}

	return fromMessageModel(model)
}

func (s *store) PutMessageLegacy(ctx context.Context, chatID *commonpb.ChatId, msg *messagingpb.Message) (*messagingpb.Message, error) {
	if msg.MessageId != nil {
		return nil, errors.New("cannot provide a message id")
	}

	msg = proto.Clone(msg).(*messagingpb.Message)
	msg.MessageId = messaging.MustGenerateMessageID()

	model, err := toLegacyMessageModel(chatID, msg)
	if err != nil {
		return nil, err
	}

	err = model.dbPut(ctx, s.pool)
	if err != nil {
		return nil, err
	}

	return fromMessageModel(model)
}

func (s *store) CountUnread(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, lastRead *messagingpb.MessageId, maxValue int64) (int64, error) {
	return dbCountUnread(ctx, s.pool, chatID, userID, lastRead, maxValue)
}

// todo: we can optimize this into one query if we change the pointer value field to a byte array
func (s *store) AdvancePointer(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId, pointer *messagingpb.Pointer) (bool, error) {
	model, err := toPointerModel(chatID, userID, pointer)
	if err != nil {
		return false, err
	}

	existing, err := dbGetPointer(ctx, s.pool, chatID, userID, int(pointer.Type))
	if err != nil {
		return false, err
	}

	if existing != nil {
		decodedExistingValue, err := pg.Decode(existing.Value)
		if err != nil {
			return false, err
		}

		// If the existing pointer is already ahead of the new pointer, don't update
		if bytes.Compare(decodedExistingValue, pointer.Value.Value) >= 0 {
			return false, nil
		}

		err = model.dbUpdate(ctx, s.pool)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	err = model.dbPut(ctx, s.pool)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *store) GetPointers(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) ([]*messagingpb.Pointer, error) {
	models, err := dbGetPointers(ctx, s.pool, chatID, userID)
	if err != nil {
		return nil, err
	}

	res := make([]*messagingpb.Pointer, len(models))
	for i, m := range models {
		userPtr, err := fromPointerModel(m)
		if err != nil {
			return nil, err
		}
		res[i] = userPtr.Pointer
	}
	return res, nil
}

func (s *store) GetAllPointers(ctx context.Context, chatID *commonpb.ChatId) ([]messaging.UserPointer, error) {
	models, err := dbGetAllPointers(ctx, s.pool, chatID)
	if err != nil {
		return nil, err
	}

	res := make([]messaging.UserPointer, len(models))
	for i, m := range models {
		res[i], err = fromPointerModel(m)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (s *store) reset() {
	_, err := s.pool.Exec(context.Background(), "DELETE FROM "+messagesTableName)
	if err != nil {
		panic(err)
	}

	_, err = s.pool.Exec(context.Background(), "DELETE FROM "+pointersTableName)
	if err != nil {
		panic(err)
	}
}
