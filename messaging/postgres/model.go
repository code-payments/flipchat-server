package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"

	pg "github.com/code-payments/flipchat-server/database/postgres"
	"github.com/code-payments/flipchat-server/messaging"
	"github.com/code-payments/flipchat-server/query"
)

const (
	messagesTableName = "flipchat_messages"
	allMessageFields  = `"id", "chatId", "senderId", "wasSenderOffStage", "version", "contentType", "content", "createdAt", "updatedAt"`

	pointersTableName = "flipchat_pointers"
	allPointerFields  = `"chatId", "userId", "type", "value", "createdAt", "updatedAt"`
)

// Message.Version enum
const (
	legacyMessageVersion  = 0
	contentMessageVersion = 1
)

// Message.ContentType enum
const (
	contentTypeUnknown                = 0
	contentTypeText                   = 1
	contentTypeLocalizedAnnouncement  = 2
	contentTypeReaction               = 5
	contentTypeReply                  = 6
	contentTypeTip                    = 7
	contentTypeDeleted                = 8
	contentTypeReview                 = 9
	contentTypeActionableAnnouncement = 10
)

type messageModel struct {
	ID                []byte    `db:"id"`
	ChatID            string    `db:"chatId"`
	SenderID          *string   `db:"senderId"`
	WasSenderOffStage bool      `db:"wasSenderOffStage"`
	Version           int       `db:"version"`
	ContentType       int       `db:"contentType"`
	Content           []byte    `db:"content"`
	CreatedAt         time.Time `db:"createdAt"`
	UpdatedAt         time.Time `db:"updatedAt"`
}

func toContentMessageModel(chatID *commonpb.ChatId, msg *messagingpb.Message) (*messageModel, error) {
	if msg.MessageId == nil {
		return nil, errors.New("message id is required")
	}

	if msg.Content == nil || len(msg.Content) != 1 {
		return nil, errors.New("unexpected content length")
	}

	var encodedSenderID *string
	if msg.SenderId != nil {
		encodedValue := pg.Encode(msg.SenderId.Value)
		encodedSenderID = &encodedValue
	}

	content := msg.Content[0]
	opaqueData, err := proto.Marshal(content)
	if err != nil {
		return nil, err
	}

	return &messageModel{
		ID:                msg.MessageId.Value,
		ChatID:            pg.Encode(chatID.Value),
		SenderID:          encodedSenderID,
		WasSenderOffStage: msg.WasSenderOffStage,
		Version:           contentMessageVersion,
		ContentType:       getContentType(content),
		Content:           opaqueData,
	}, nil
}

func toLegacyMessageModel(chatID *commonpb.ChatId, msg *messagingpb.Message) (*messageModel, error) {
	if msg.MessageId == nil {
		return nil, errors.New("message id is required")
	}

	var encodedSenderID *string
	if msg.SenderId != nil {
		encodedValue := pg.Encode(msg.SenderId.Value)
		encodedSenderID = &encodedValue
	}

	opaqueData, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	return &messageModel{
		ID:                msg.MessageId.Value,
		ChatID:            pg.Encode(chatID.Value),
		SenderID:          encodedSenderID,
		WasSenderOffStage: msg.WasSenderOffStage,
		Version:           legacyMessageVersion,
		ContentType:       contentTypeUnknown,
		Content:           opaqueData,
	}, nil
}

func fromMessageModel(m *messageModel) (*messagingpb.Message, error) {
	// For legacy messages, we just unmarshal the content as a messagingpb.Message
	if m.Version == legacyMessageVersion {
		protoMessage := &messagingpb.Message{}
		err := proto.Unmarshal(m.Content, protoMessage)
		if err != nil {
			return nil, err
		}

		var ts *timestamppb.Timestamp
		if !m.CreatedAt.IsZero() {
			ts = timestamppb.New(m.CreatedAt)
			protoMessage.Ts = ts
		}

		return protoMessage, nil

		// For content messages, we unmarshal the content as a messagingpb.Content
	} else if m.Version == contentMessageVersion {
		protoContent := &messagingpb.Content{}

		err := proto.Unmarshal(m.Content, protoContent)
		if err != nil {
			return nil, err
		}

		var ts *timestamppb.Timestamp
		if !m.CreatedAt.IsZero() {
			ts = timestamppb.New(m.CreatedAt)
		}

		protoMessage := &messagingpb.Message{
			MessageId:         &messagingpb.MessageId{Value: m.ID},
			Content:           []*messagingpb.Content{protoContent},
			Ts:                ts,
			WasSenderOffStage: m.WasSenderOffStage,
		}

		if m.SenderID != nil {
			decodedSenderId, err := pg.Decode(*m.SenderID)
			if err != nil {
				return nil, err
			}
			protoMessage.SenderId = &commonpb.UserId{Value: decodedSenderId}
		}

		return protoMessage, nil

	} else {
		return nil, errors.New("unknown message version")
	}
}

type pointerModel struct {
	ChatID    string    `db:"chatId"`
	UserID    string    `db:"userId"`
	Type      int       `db:"type"`
	Value     string    `db:"value"`
	CreatedAt time.Time `db:"createdAt"`
	UpdatedAt time.Time `db:"updatedAt"`
}

func toPointerModel(chatID *commonpb.ChatId, userID *commonpb.UserId, pointer *messagingpb.Pointer) (*pointerModel, error) {
	return &pointerModel{
		ChatID: pg.Encode(chatID.Value),
		UserID: pg.Encode(userID.Value),
		Type:   int(pointer.Type),
		Value:  pg.Encode(pointer.Value.Value),
	}, nil
}

func fromPointerModel(m *pointerModel) (messaging.UserPointer, error) {
	decodedUserID, err := pg.Decode(m.UserID)
	if err != nil {
		return messaging.UserPointer{}, err
	}

	decodedValue, err := pg.Decode(m.Value)
	if err != nil {
		return messaging.UserPointer{}, err
	}

	return messaging.UserPointer{
		UserID: &commonpb.UserId{Value: decodedUserID},
		Pointer: &messagingpb.Pointer{
			Type:  messagingpb.Pointer_Type(m.Type),
			Value: &messagingpb.MessageId{Value: decodedValue},
		},
	}, nil
}

func (m *messageModel) dbPut(ctx context.Context, pool *pgxpool.Pool) error {
	query := `INSERT INTO ` + messagesTableName + `(` + allMessageFields + `) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW()) RETURNING ` + allMessageFields
	return pgxscan.Get(
		ctx,
		pool,
		m,
		query,
		m.ID,
		m.ChatID,
		m.SenderID,
		m.WasSenderOffStage,
		m.Version,
		m.ContentType,
		m.Content,
	)
}

func dbGetMessage(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, messageID *messagingpb.MessageId) (*messageModel, error) {
	res := &messageModel{}
	query := `SELECT ` + allMessageFields + ` FROM ` + messagesTableName + ` WHERE "chatId" = $1 AND "id" = $2`
	err := pgxscan.Get(
		ctx,
		pool,
		res,
		query,
		pg.Encode(chatID.Value),
		messageID.Value,
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, messaging.ErrMessageNotFound
		}
		return nil, err
	}
	return res, nil
}

func dbGetBatchMessages(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, messageIDs ...*messagingpb.MessageId) ([]*messageModel, error) {
	var res []*messageModel

	queryParameters := make([]any, len(messageIDs)+1)
	queryParameters[0] = pg.Encode(chatID.Value)

	query := `SELECT ` + allMessageFields + ` FROM ` + messagesTableName + ` WHERE "chatId" = $1 AND "id" IN (`
	for i, messageID := range messageIDs {
		queryParameters[i+1] = messageID.Value
		if i > 0 {
			query += fmt.Sprintf(",$%d", i+2)
		} else {
			query += fmt.Sprintf("$%d", i+2)
		}
	}
	query += ")"

	err := pgxscan.Select(
		ctx,
		pool,
		&res,
		query,
		queryParameters...,
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return res, nil
}

func dbGetPagedMessages(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, options ...query.Option) ([]*messageModel, error) {
	var res []*messageModel

	appliedOptions := query.ApplyOptions(options...)
	queryParameters := []any{pg.Encode(chatID.Value)}
	query := `SELECT ` + allMessageFields + ` FROM ` + messagesTableName + ` WHERE "chatId" = $1`

	if appliedOptions.Token != nil {
		queryParameters = append(queryParameters, appliedOptions.Token.Value)
		if appliedOptions.Order == commonpb.QueryOptions_ASC {
			query += fmt.Sprintf(` AND "id" > $%d`, len(queryParameters))
		} else {
			query += fmt.Sprintf(` AND "id" < $%d`, len(queryParameters))
		}
	}

	if appliedOptions.Order == commonpb.QueryOptions_ASC {
		query += ` ORDER BY "id" ASC`
	} else {
		query += ` ORDER BY "id" DESC`
	}

	if appliedOptions.Limit > 0 {
		queryParameters = append(queryParameters, appliedOptions.Limit)
		query += fmt.Sprintf(` LIMIT $%d`, len(queryParameters))
	}

	err := pgxscan.Select(
		ctx,
		pool,
		&res,
		query,
		queryParameters...,
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return res, nil
}

func dbCountUnread(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, userID *commonpb.UserId, lastRead *messagingpb.MessageId, maxValue int64) (int64, error) {
	var res int64
	queryParameters := []any{pg.Encode(chatID.Value), pg.Encode(userID.Value)}
	query := `SELECT COUNT(*) FROM (SELECT "id" FROM ` + messagesTableName + ` WHERE "chatId" = $1 AND ("senderId" IS NULL OR "senderId" != $2) AND "contentType" IN (0, 1, 6)`
	if lastRead != nil {
		queryParameters = append(queryParameters, lastRead.Value)
		query += fmt.Sprintf(`AND "id" > $%d`, len(queryParameters))
	}
	if maxValue >= 0 {
		queryParameters = append(queryParameters, maxValue)
		query += fmt.Sprintf(` LIMIT $%d`, len(queryParameters))
	}
	query += ") AS counted"
	err := pgxscan.Get(
		ctx,
		pool,
		&res,
		query,
		queryParameters...,
	)
	return res, err
}

func (m *pointerModel) dbPut(ctx context.Context, pool *pgxpool.Pool) error {
	query := `INSERT INTO ` + pointersTableName + `(` + allPointerFields + `) VALUES ($1, $2, $3, $4, now(), now()) RETURNING ` + allPointerFields
	return pgxscan.Get(
		ctx,
		pool,
		m,
		query,
		m.ChatID,
		m.UserID,
		m.Type,
		m.Value,
	)
}

func (m *pointerModel) dbUpdate(ctx context.Context, pool *pgxpool.Pool) error {
	query := `UPDATE ` + pointersTableName + ` SET "value" = $1, "updatedAt" = now() WHERE "chatId" = $2 AND "userId" = $3 and "type" = $4 RETURNING ` + allPointerFields
	return pgxscan.Get(
		ctx,
		pool,
		m,
		query,
		m.Value,
		m.ChatID,
		m.UserID,
		m.Type,
	)
}

func dbGetPointer(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, userID *commonpb.UserId, pointerType int) (*pointerModel, error) {
	res := &pointerModel{}
	query := `SELECT ` + allPointerFields + ` FROM ` + pointersTableName + ` WHERE "chatId" = $1 AND "userId" = $2 and "type" = $3`
	err := pgxscan.Get(
		ctx,
		pool,
		res,
		query,
		pg.Encode(chatID.Value),
		pg.Encode(userID.Value),
		pointerType,
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return res, nil
}

func dbGetPointers(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, userID *commonpb.UserId) ([]*pointerModel, error) {
	var res []*pointerModel
	query := `SELECT ` + allPointerFields + ` FROM ` + pointersTableName + ` WHERE "chatId" = $1 AND "userId" = $2`
	err := pgxscan.Select(
		ctx,
		pool,
		&res,
		query,
		pg.Encode(chatID.Value),
		pg.Encode(userID.Value),
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return res, nil
}

func dbGetAllPointers(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId) ([]*pointerModel, error) {
	var res []*pointerModel
	query := `SELECT ` + allPointerFields + ` FROM ` + pointersTableName + ` WHERE "chatId" = $1`
	err := pgxscan.Select(
		ctx,
		pool,
		&res,
		query,
		pg.Encode(chatID.Value),
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return res, nil
}

func getContentType(content *messagingpb.Content) int {
	switch content.Type.(type) {
	case *messagingpb.Content_Text:
		return contentTypeText
	case *messagingpb.Content_LocalizedAnnouncement:
		return contentTypeLocalizedAnnouncement
	case *messagingpb.Content_Reaction:
		return contentTypeReaction
	case *messagingpb.Content_Reply:
		return contentTypeReply
	case *messagingpb.Content_Tip:
		return contentTypeTip
	case *messagingpb.Content_Deleted:
		return contentTypeDeleted
	case *messagingpb.Content_Review:
		return contentTypeReview
	case *messagingpb.Content_ActionableAnnouncement:
		return contentTypeActionableAnnouncement
	default:
		return contentTypeUnknown
	}
}
