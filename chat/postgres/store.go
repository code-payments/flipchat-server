package postgres

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/chat"
)

type store struct {
	pool *pgxpool.Pool
}

func NewInPostgres(pool *pgxpool.Pool) chat.Store {
	return &store{
		pool: pool,
	}
}

func (s *store) GetChatID(ctx context.Context, roomID uint64) (*commonpb.ChatId, error) {
	return dbGetChatID(ctx, s.pool, roomID)
}

func (s *store) GetChatMetadata(ctx context.Context, chatID *commonpb.ChatId) (*chatpb.Metadata, error) {
	model, err := dbGetChatMetadata(ctx, s.pool, chatID)
	if err != nil {
		return nil, err
	}
	return fromChatModel(model)
}

func (s *store) GetChatMetadataBatched(ctx context.Context, chatIDs ...*commonpb.ChatId) ([]*chatpb.Metadata, error) {
	models, err := dbGetChatMetadataBatched(ctx, s.pool, chatIDs...)
	if err != nil {
		return nil, err
	}
	res := make([]*chatpb.Metadata, len(models))
	for i, model := range models {
		res[i], err = fromChatModel(model)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (s *store) GetChatsForUser(ctx context.Context, userID *commonpb.UserId) ([]*commonpb.ChatId, error) {
	return dbGetChatsForUser(ctx, s.pool, userID)
}

func (s *store) GetMembers(ctx context.Context, chatID *commonpb.ChatId) ([]*chat.Member, error) {
	models, err := dbGetMembers(ctx, s.pool, chatID)
	if err != nil {
		return nil, err
	}

	res := make([]*chat.Member, len(models))
	for i, model := range models {
		res[i], err = fromMemberModel(model)
		if err != nil {
			return nil, err
		}
	}
	slices.SortFunc(res, func(a, b *chat.Member) int {
		return bytes.Compare(a.UserID.Value, b.UserID.Value)
	})
	return res, nil
}

func (s *store) GetMember(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (*chat.Member, error) {
	model, err := dbGetMember(ctx, s.pool, chatID, userID)
	if err != nil {
		return nil, err
	}
	return fromMemberModel(model)
}

func (s *store) IsMember(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	return dbIsMember(ctx, s.pool, chatID, userID)
}

func (s *store) CreateChat(ctx context.Context, md *chatpb.Metadata) (*chatpb.Metadata, error) {
	if md.ChatId == nil {
		return nil, errors.New("must provide chat id")
	}
	if md.RoomNumber != 0 {
		return nil, errors.New("cannot create chat with room number")
	}

	existing, err := dbGetChatMetadata(ctx, s.pool, md.ChatId)
	if err == nil {
		res, err := fromChatModel(existing)
		if err != nil {
			return nil, err
		}
		return res, chat.ErrChatExists
	} else if err != chat.ErrChatNotFound {
		return nil, err
	}

	chatModel, err := toChatModel(md)
	if err != nil {
		return nil, err
	}

	err = chatModel.dbPut(ctx, s.pool)
	if err != nil {
		return nil, err
	}

	if md.Owner != nil && false {
		ownerMember := &chat.Member{
			UserID:            md.Owner,
			HasModPermission:  true,
			HasSendPermission: true,
		}

		ownerModel, err := toMemberModel(md.ChatId, ownerMember)
		if err != nil {
			return nil, err
		}

		err = ownerModel.dbAdd(ctx, s.pool)
		if err != nil {
			return nil, err
		}
	}

	return fromChatModel(chatModel)
}

func (s *store) AddMember(ctx context.Context, chatID *commonpb.ChatId, member chat.Member) error {
	model, err := toMemberModel(chatID, &member)
	if err != nil {
		return err
	}
	return model.dbAdd(ctx, s.pool)
}

func (s *store) RemoveMember(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) error {
	return dbRemoveMember(ctx, s.pool, chatID, member)
}

func (s *store) SetDisplayName(ctx context.Context, chatID *commonpb.ChatId, displayName string) error {
	return dbSetDisplayName(ctx, s.pool, chatID, displayName)
}

func (s *store) SetMessagingFee(ctx context.Context, chatID *commonpb.ChatId, messagingFee *commonpb.PaymentAmount) error {
	return dbSetMessagingFee(ctx, s.pool, chatID, messagingFee)
}

func (s *store) SetOpenStatus(ctx context.Context, chatID *commonpb.ChatId, isOpen bool) error {
	return dbSetOpenStatus(ctx, s.pool, chatID, isOpen)
}

func (s *store) SetMuteState(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId, isMuted bool) error {
	return dbSetMuteState(ctx, s.pool, chatID, member, isMuted)
}

func (s *store) IsUserMuted(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) (bool, error) {
	return dbIsUserMuted(ctx, s.pool, chatID, member)
}

func (s *store) SetSendPermission(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId, hasSendPermission bool) error {
	return dbSetSendPermission(ctx, s.pool, chatID, member, hasSendPermission)
}

func (s *store) HasSendPermission(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) (bool, error) {
	return dbHasSendPermission(ctx, s.pool, chatID, member)
}

func (s *store) SetPushState(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId, isPushEnabled bool) error {
	return dbSetPushState(ctx, s.pool, chatID, member, isPushEnabled)
}

func (s *store) IsPushEnabled(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) (bool, error) {
	return dbIsPushEnabled(ctx, s.pool, chatID, member)
}

func (s *store) AdvanceLastChatActivity(ctx context.Context, chatID *commonpb.ChatId, ts time.Time) error {
	return dbAdvanceLastChatActivity(ctx, s.pool, chatID, ts)
}

func (s *store) reset() {
	_, err := s.pool.Exec(context.Background(), "DELETE FROM "+membersTableName)
	if err != nil {
		panic(err)
	}

	_, err = s.pool.Exec(context.Background(), "DELETE FROM "+chatsTableName)
	if err != nil {
		panic(err)
	}
}
