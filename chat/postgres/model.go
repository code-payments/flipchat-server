package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/flipchat-server/chat"
	pg "github.com/code-payments/flipchat-server/database/postgres"
)

const (
	chatsTableName = "flipchat_chats"
	allChatFields  = `"id", "displayName", "roomNumber", "coverCharge", "type", "isOpen", "description", "createdBy", "createdAt", "updatedAt", "lastActivityAt"`

	membersTableName = "flipchat_members"
	allMemberFields  = `"chatId", "userId", "addedById", "isMuted", "isPushEnabled", "hasModPermission", "hasSendPermission", "isSoftDeleted", "createdAt", "updatedAt"`
)

type chatModel struct {
	ID             string    `db:"id"`
	DisplayName    *string   `db:"displayName"`
	RoomNumber     uint64    `db:"roomNumber"`
	CoverCharge    uint64    `db:"coverCharge"`
	Type           int       `db:"type"`
	IsOpen         bool      `db:"isOpen"`
	Descripton     *string   `db:"description"`
	CreatedBy      string    `db:"createdBy"`
	CreatedAt      time.Time `db:"createdAt"`
	UpdatedAt      time.Time `db:"updatedAt"`
	LastActivityAt time.Time `db:"lastActivityAt"`
}

type memberModel struct {
	ChatID            string    `db:"chatId"`
	UserID            string    `db:"userId"`
	AddedById         *string   `db:"addedById"`
	IsMuted           bool      `db:"isMuted"`
	IsPushEnabled     bool      `db:"isPushEnabled"`
	HasModPermission  bool      `db:"hasModPermission"`
	HasSendPermission bool      `db:"hasSendPermission"`
	IsSoftDeleted     bool      `db:"isSoftDeleted"`
	CreatedAt         time.Time `db:"createdAt"`
	UpdatedAt         time.Time `db:"updatedAt"`
}

func toChatModel(obj *chatpb.Metadata) (*chatModel, error) {
	var displayName *string
	if len(obj.DisplayName) > 0 {
		value := obj.DisplayName
		displayName = &value
	}

	var coverCharge uint64
	if obj.MessagingFee != nil {
		coverCharge = obj.MessagingFee.Quarks
	}

	isOpen := true
	if obj.OpenStatus != nil {
		isOpen = obj.OpenStatus.IsCurrentlyOpen
	}

	var description *string
	if len(obj.Description) > 0 {
		value := obj.Description
		description = &value
	}

	var createdBy string
	if obj.Owner != nil {
		createdBy = pg.Encode(obj.Owner.Value)
	}

	return &chatModel{
		ID:             pg.Encode(obj.ChatId.Value),
		DisplayName:    displayName,
		RoomNumber:     obj.RoomNumber,
		CoverCharge:    coverCharge,
		Type:           int(obj.Type),
		IsOpen:         isOpen,
		Descripton:     description,
		CreatedBy:      createdBy,
		LastActivityAt: obj.LastActivity.AsTime().UTC(),
	}, nil
}

func fromChatModel(m *chatModel) (*chatpb.Metadata, error) {
	decodedChatID, err := pg.Decode(m.ID)
	if err != nil {
		return nil, err
	}

	var owner *commonpb.UserId
	if len(m.CreatedBy) > 0 {
		decodedOwner, err := pg.Decode(m.CreatedBy)
		if err != nil {
			return nil, err
		}
		owner = &commonpb.UserId{Value: decodedOwner}
	}

	var messagingFee *commonpb.PaymentAmount
	if m.CoverCharge > 0 {
		messagingFee = &commonpb.PaymentAmount{Quarks: m.CoverCharge}
	}

	return &chatpb.Metadata{
		ChatId:       &commonpb.ChatId{Value: decodedChatID},
		DisplayName:  *pointer.StringOrDefault(m.DisplayName, ""),
		RoomNumber:   m.RoomNumber,
		MessagingFee: messagingFee,
		Type:         chatpb.Metadata_ChatType(m.Type),
		OpenStatus:   &chatpb.OpenStatus{IsCurrentlyOpen: m.IsOpen},
		Description:  *pointer.StringOrDefault(m.Descripton, ""),
		Owner:        owner,
		LastActivity: timestamppb.New(m.LastActivityAt),
	}, nil
}

func toMemberModel(chatID *commonpb.ChatId, member *chat.Member) (*memberModel, error) {
	var addedByID *string
	if member.AddedBy != nil {
		value := pg.Encode(member.AddedBy.Value)
		addedByID = &value
	}

	return &memberModel{
		ChatID:            pg.Encode(chatID.Value),
		UserID:            pg.Encode(member.UserID.Value),
		AddedById:         addedByID,
		IsMuted:           member.IsMuted,
		IsPushEnabled:     member.IsPushEnabled,
		HasModPermission:  member.HasModPermission,
		HasSendPermission: member.HasSendPermission,
		IsSoftDeleted:     member.IsSoftDeleted,
	}, nil
}

func fromMemberModel(m *memberModel) (*chat.Member, error) {
	decodedUserID, err := pg.Decode(m.UserID)
	if err != nil {
		return nil, err
	}

	var addedBy *commonpb.UserId
	if m.AddedById != nil {
		decodedAddedByID, err := pg.Decode(*m.AddedById)
		if err != nil {
			return nil, err
		}
		addedBy = &commonpb.UserId{Value: decodedAddedByID}
	}

	return &chat.Member{
		UserID:            &commonpb.UserId{Value: decodedUserID},
		AddedBy:           addedBy,
		IsMuted:           m.IsMuted,
		IsPushEnabled:     m.IsPushEnabled,
		HasModPermission:  m.HasModPermission,
		HasSendPermission: m.HasSendPermission,
		IsSoftDeleted:     m.IsSoftDeleted,
	}, nil
}

func (m *chatModel) dbPut(ctx context.Context, pool *pgxpool.Pool) error {
	var largest uint64
	getLargestRoomNumberQuery := `SELECT COALESCE(MAX("roomNumber"), 0) FROM ` + chatsTableName
	err := pgxscan.Get(
		ctx,
		pool,
		&largest,
		getLargestRoomNumberQuery,
	)
	if err == nil {
		m.RoomNumber = largest + 1
	} else if pgxscan.NotFound(err) {
		m.RoomNumber = 1
	} else {
		return err
	}

	putQuery := `INSERT INTO ` + chatsTableName + `(` + allChatFields + `) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW(), $9) RETURNING ` + allChatFields
	err = pgxscan.Get(
		ctx,
		pool,
		m,
		putQuery,
		m.ID,
		m.DisplayName,
		m.RoomNumber,
		m.CoverCharge,
		m.Type,
		m.IsOpen,
		m.Descripton,
		m.CreatedBy,
		m.LastActivityAt,
	)
	if err != nil {
		return err
	}

	return nil
}

func (m *memberModel) dbAdd(ctx context.Context, pool *pgxpool.Pool) error {
	m.IsPushEnabled = true
	m.IsSoftDeleted = false

	existsResult := struct {
		IsMuted       bool `db:"isMuted"`
		IsSoftDeleted bool `db:"isSoftDeleted"`
	}{}
	existsQuery := `SELECT "isMuted", "isSoftDeleted" FROM ` + membersTableName + ` WHERE "chatId" = $1 AND "userId" = $2`
	err := pgxscan.Get(
		ctx,
		pool,
		&existsResult,
		existsQuery,
		m.ChatID,
		m.UserID,
	)
	if err == nil {
		if !existsResult.IsSoftDeleted {
			return nil
		}
		m.IsMuted = existsResult.IsMuted
	} else if !pgxscan.NotFound(err) {
		return err
	}

	addQuery := `INSERT INTO ` + membersTableName + `(` + allMemberFields + `) VALUES ($1, $2, $3, $4, $5, $6, $7, false, NOW(), NOW()) RETURNING ` + allMemberFields
	addQueryParameters := []any{m.ChatID, m.UserID, m.AddedById, m.IsMuted, m.IsPushEnabled, m.HasModPermission, m.HasSendPermission}
	if existsResult.IsSoftDeleted {
		addQuery = `UPDATE ` + membersTableName + ` SET "isSoftDeleted" = $1, "isPushEnabled" = $2, "isMuted" = $3, "hasModPermission" = $4, "hasSendPermission" = $5, "addedById" = $6, "updatedAt" = NOW() WHERE "chatId" = $7 AND "userId" = $8 RETURNING ` + allMemberFields
		addQueryParameters = []any{m.IsSoftDeleted, m.IsPushEnabled, m.IsMuted, m.HasModPermission, m.HasSendPermission, m.AddedById, m.ChatID, m.UserID}
	}
	err = pgxscan.Get(
		ctx,
		pool,
		m,
		addQuery,
		addQueryParameters...,
	)
	if err != nil {
		return err
	}
	return nil
}

func dbGetChatID(ctx context.Context, pool *pgxpool.Pool, roomID uint64) (*commonpb.ChatId, error) {
	var encodedChatID string
	query := `SELECT "id" FROM ` + chatsTableName + ` WHERE "roomNumber" = $1`
	err := pgxscan.Get(
		ctx,
		pool,
		&encodedChatID,
		query,
		roomID,
	)
	if pgxscan.NotFound(err) {
		return nil, chat.ErrChatNotFound
	}
	decodedChatID, err := pg.Decode(encodedChatID)
	if err != nil {
		return nil, err
	}
	return &commonpb.ChatId{Value: decodedChatID}, err
}

func dbGetChatMetadata(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId) (*chatModel, error) {
	res := &chatModel{}
	query := `SELECT ` + allChatFields + ` FROM ` + chatsTableName + ` WHERE "id" = $1`
	err := pgxscan.Get(
		ctx,
		pool,
		res,
		query,
		pg.Encode(chatID.Value),
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, chat.ErrChatNotFound
		}
		return nil, err
	}
	return res, nil
}

func dbGetChatMetadataBatched(ctx context.Context, pool *pgxpool.Pool, chatIDs ...*commonpb.ChatId) ([]*chatModel, error) {
	if len(chatIDs) == 0 {
		return nil, nil
	}

	var res []*chatModel

	queryParameters := make([]any, len(chatIDs))
	query := `SELECT ` + allChatFields + ` FROM ` + chatsTableName + ` WHERE "id" IN (`
	for i, chatID := range chatIDs {
		queryParameters[i] = pg.Encode(chatID.Value)
		if i > 0 {
			query += fmt.Sprintf(",$%d", i+1)
		} else {
			query += fmt.Sprintf("$%d", i+1)
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
			return nil, chat.ErrChatNotFound
		}
		return nil, err
	}
	if len(res) != len(chatIDs) {
		return nil, chat.ErrChatNotFound
	}
	return res, nil
}

func dbGetChatsForUser(ctx context.Context, pool *pgxpool.Pool, userID *commonpb.UserId) ([]*commonpb.ChatId, error) {
	var encodedChatIDs []string
	query := `SELECT "chatId" FROM ` + membersTableName + ` WHERE "userId" = $1 AND NOT "isSoftDeleted"`
	err := pgxscan.Select(
		ctx,
		pool,
		&encodedChatIDs,
		query,
		pg.Encode(userID.Value),
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	res := make([]*commonpb.ChatId, len(encodedChatIDs))
	for i, encodedChatID := range encodedChatIDs {
		decodedChatID, err := pg.Decode(encodedChatID)
		if err != nil {
			return nil, err
		}
		res[i] = &commonpb.ChatId{Value: decodedChatID}
	}
	return res, nil
}

func dbGetMembers(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId) ([]*memberModel, error) {
	var res []*memberModel
	query := `SELECT ` + allMemberFields + ` FROM ` + membersTableName + ` WHERE "chatId" = $1 AND NOT "isSoftDeleted"`
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

func dbGetMember(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, userID *commonpb.UserId) (*memberModel, error) {
	res := &memberModel{}
	query := `SELECT ` + allMemberFields + ` FROM ` + membersTableName + ` WHERE "chatId" = $1 AND "userId" = $2 AND NOT "isSoftDeleted"`
	err := pgxscan.Get(
		ctx,
		pool,
		res,
		query,
		pg.Encode(chatID.Value),
		pg.Encode(userID.Value),
	)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, chat.ErrChatNotFound
		}
		return nil, err
	}
	return res, nil
}

func dbIsMember(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM ` + membersTableName + ` WHERE "chatId" = $1 AND "userId" = $2 AND NOT "isSoftDeleted"`
	err := pgxscan.Get(
		ctx,
		pool,
		&count,
		query,
		pg.Encode(chatID.Value),
		pg.Encode(userID.Value),
	)
	if err != nil {
		return false, err
	}
	return count == 1, nil
}

func dbRemoveMember(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, member *commonpb.UserId) error {
	query := `UPDATE ` + membersTableName + ` SET "isSoftDeleted" = true, "addedById" = NULL, "isPushEnabled" = true, "hasModPermission" = false, "hasSendPermission" = false, "updatedAt" = NOW() WHERE "chatId" = $1 AND "userId" = $2 AND NOT "isSoftDeleted"`
	_, err := pool.Exec(
		ctx,
		query,
		pg.Encode(chatID.Value),
		pg.Encode(member.Value),
	)
	return err
}

func dbSetDisplayName(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, displayName string) error {
	query := `UPDATE ` + chatsTableName + ` SET "displayName" = $1, "updatedAt" = NOW() WHERE "id" = $2`
	queryParameters := []any{displayName, pg.Encode(chatID.Value)}
	if len(displayName) == 0 {
		query = `UPDATE ` + chatsTableName + ` SET "displayName" = NULL, "updatedAt" = NOW() WHERE "id" = $1`
		queryParameters = []any{pg.Encode(chatID.Value)}
	}
	res, err := pool.Exec(
		ctx,
		query,
		queryParameters...,
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return chat.ErrChatNotFound
	}
	return nil
}

func dbSetDescription(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, description string) error {
	query := `UPDATE ` + chatsTableName + ` SET "description" = $1, "updatedAt" = NOW() WHERE "id" = $2`
	queryParameters := []any{description, pg.Encode(chatID.Value)}
	if len(description) == 0 {
		query = `UPDATE ` + chatsTableName + ` SET "description" = NULL, "updatedAt" = NOW() WHERE "id" = $1`
		queryParameters = []any{pg.Encode(chatID.Value)}
	}
	res, err := pool.Exec(
		ctx,
		query,
		queryParameters...,
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return chat.ErrChatNotFound
	}
	return nil
}

func dbSetMessagingFee(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, messagingFee *commonpb.PaymentAmount) error {
	query := `UPDATE ` + chatsTableName + ` SET "coverCharge" = $1, "updatedAt" = NOW() WHERE "id" = $2`
	res, err := pool.Exec(
		ctx,
		query,
		messagingFee.Quarks,
		pg.Encode(chatID.Value),
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return chat.ErrChatNotFound
	}
	return nil
}

func dbSetOpenStatus(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, isOpen bool) error {
	query := `UPDATE ` + chatsTableName + ` SET "isOpen" = $1, "updatedAt" = NOW() WHERE "id" = $2`
	res, err := pool.Exec(
		ctx,
		query,
		isOpen,
		pg.Encode(chatID.Value),
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return chat.ErrChatNotFound
	}
	return nil
}

func dbSetMuteState(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, member *commonpb.UserId, isMuted bool) error {
	query := `UPDATE ` + membersTableName + ` SET "isMuted" = $1, "updatedAt" = NOW() WHERE "chatId" = $2 AND "userId" = $3 AND NOT "isSoftDeleted"`
	res, err := pool.Exec(
		ctx,
		query,
		isMuted,
		pg.Encode(chatID.Value),
		pg.Encode(member.Value),
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return chat.ErrMemberNotFound
	}
	return nil
}

func dbIsUserMuted(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, member *commonpb.UserId) (bool, error) {
	var res bool
	query := `SELECT "isMuted" FROM ` + membersTableName + ` WHERE "chatId" = $1 AND "userId" = $2 AND NOT "isSoftDeleted"`
	err := pgxscan.Get(
		ctx,
		pool,
		&res,
		query,
		pg.Encode(chatID.Value),
		pg.Encode(member.Value),
	)
	if pgxscan.NotFound(err) {
		return false, chat.ErrMemberNotFound
	}
	return res, err
}

func dbSetSendPermission(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, member *commonpb.UserId, hasSendPermission bool) error {
	query := `UPDATE ` + membersTableName + ` SET "hasSendPermission" = $1, "updatedAt" = NOW() WHERE "chatId" = $2 AND "userId" = $3 AND NOT "isSoftDeleted"`
	res, err := pool.Exec(
		ctx,
		query,
		hasSendPermission,
		pg.Encode(chatID.Value),
		pg.Encode(member.Value),
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return chat.ErrMemberNotFound
	}
	return nil
}

func dbHasSendPermission(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, member *commonpb.UserId) (bool, error) {
	var res bool
	query := `SELECT "hasSendPermission" FROM ` + membersTableName + ` WHERE "chatId" = $1 AND "userId" = $2 AND NOT "isSoftDeleted"`
	err := pgxscan.Get(
		ctx,
		pool,
		&res,
		query,
		pg.Encode(chatID.Value),
		pg.Encode(member.Value),
	)
	if pgxscan.NotFound(err) {
		return false, chat.ErrMemberNotFound
	}
	return res, err
}

func dbSetPushState(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, member *commonpb.UserId, isPushEnabled bool) error {
	query := `UPDATE ` + membersTableName + ` SET "isPushEnabled" = $1, "updatedAt" = NOW() WHERE "chatId" = $2 AND "userId" = $3 AND NOT "isSoftDeleted"`
	res, err := pool.Exec(
		ctx,
		query,
		isPushEnabled,
		pg.Encode(chatID.Value),
		pg.Encode(member.Value),
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return chat.ErrMemberNotFound
	}
	return nil
}

func dbIsPushEnabled(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, member *commonpb.UserId) (bool, error) {
	var res bool
	query := `SELECT "isPushEnabled" FROM ` + membersTableName + ` WHERE "chatId" = $1 AND "userId" = $2`
	err := pgxscan.Get(
		ctx,
		pool,
		&res,
		query,
		pg.Encode(chatID.Value),
		pg.Encode(member.Value),
	)
	if pgxscan.NotFound(err) {
		return false, chat.ErrMemberNotFound
	}
	return res, err
}

func dbAdvanceLastChatActivity(ctx context.Context, pool *pgxpool.Pool, chatID *commonpb.ChatId, ts time.Time) error {
	query := `UPDATE ` + chatsTableName + ` SET "lastActivityAt" = $1, "updatedAt" = NOW() WHERE "id" = $2`
	res, err := pool.Exec(
		ctx,
		query,
		ts.UTC(),
		pg.Encode(chatID.Value),
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return chat.ErrChatNotFound
	}
	return nil
}
