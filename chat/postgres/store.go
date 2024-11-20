package postgres

import (
	"bytes"
	"context"
	"errors"
	"slices"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
	"google.golang.org/protobuf/proto"

	"github.com/code-payments/flipchat-server/chat"
	pg "github.com/code-payments/flipchat-server/database/postgres"
	"github.com/code-payments/flipchat-server/database/prisma/db"
	"github.com/code-payments/flipchat-server/query"
)

type store struct {
	client *db.PrismaClient
}

func NewPostgres(client *db.PrismaClient) chat.Store {
	return &store{
		client,
	}
}

func (s *store) reset() {
	ctx := context.Background()

	members := s.client.Member.FindMany().Delete().Tx()
	chats := s.client.Chat.FindMany().Delete().Tx()

	err := s.client.Prisma.Transaction(members, chats).Exec(ctx)
	if err != nil {
		panic(err)
	}
}

func fromModelWithOwner(m *db.ChatModel, owner *commonpb.UserId) (*chatpb.Metadata, error) {
	meta, err := fromModel(m)
	if err != nil {
		return nil, err
	}

	meta.Owner = proto.Clone(owner).(*commonpb.UserId)

	return meta, nil
}

func fromModel(m *db.ChatModel) (*chatpb.Metadata, error) {
	decodedChatID, err := pg.Decode(m.ID)

	if err != nil {
		return nil, err
	}

	room := uint64(0)
	if roomNumber, ok := m.RoomNumber(); ok {
		room = uint64(roomNumber)
	}

	coverCharge := (*commonpb.PaymentAmount)(nil)
	if m.CoverCharge != 0 {
		coverCharge = &commonpb.PaymentAmount{Quarks: uint64(m.CoverCharge)}
	}

	return &chatpb.Metadata{
		ChatId: &commonpb.ChatId{Value: decodedChatID},

		Type:       chatpb.Metadata_ChatType(m.Type),
		Title:      m.Title,
		RoomNumber: room,

		IsMuted:   false, // not stored in the DB on this model
		Muteable:  true,  // not stored in the DB on this model
		NumUnread: 0,     // not stored in the DB on this model

		CoverCharge: coverCharge,
	}, nil
}

func (s *store) GetChatID(ctx context.Context, roomID uint64) (*commonpb.ChatId, error) {
	res, err := s.client.Chat.FindUnique(
		db.Chat.RoomNumber.Equals(int(roomID)),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) || res == nil {
		return nil, chat.ErrChatNotFound
	} else if err != nil {
		return nil, err
	}

	val, err := pg.Decode(res.ID)
	if err != nil {
		return nil, err
	}

	return &commonpb.ChatId{Value: val}, nil
}

func (s *store) GetChatMetadata(ctx context.Context, chatID *commonpb.ChatId) (*chatpb.Metadata, error) {
	encodedChatID := pg.Encode(chatID.Value)

	// Find the room
	res, err := s.client.Chat.FindUnique(
		db.Chat.ID.Equals(encodedChatID),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) || res == nil {
		return nil, chat.ErrChatNotFound
	} else if err != nil {
		return nil, err
	}

	// Find the owner (host), currently assumed to only be one
	owner, err := s.client.Member.FindFirst(
		db.Member.ChatID.Equals(encodedChatID),
		db.Member.IsHost.Equals(true),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) || owner == nil {
		return fromModel(res)
	} else if err != nil {
		return nil, err
	}

	decodedOwnerID, err := pg.Decode(owner.UserID)
	if err != nil {
		return nil, err
	}

	return fromModelWithOwner(res, &commonpb.UserId{Value: decodedOwnerID})
}

func (s *store) GetChatsForUser(ctx context.Context, userID *commonpb.UserId, opts ...query.Option) ([]*commonpb.ChatId, error) {

	encodedUserID := pg.Encode(userID.Value)

	res, err := s.client.Member.FindMany(
		db.Member.UserID.Equals(encodedUserID),
	).Select(
		db.Member.ChatID.Field(),
	).Exec(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Use the DB to sort, limit, and paginate the results
	// (Using the same logic from the in-memory store for now)

	var chatIDs []*commonpb.ChatId
	for _, member := range res {
		decodedChatID, err := pg.Decode(member.ChatID)
		if err != nil {
			return nil, err
		}

		chatIDs = append(chatIDs, &commonpb.ChatId{Value: decodedChatID})
	}

	queryOpts := query.DefaultOptions()
	for _, o := range opts {
		o(&queryOpts)
	}

	slices.SortFunc(chatIDs, func(a, b *commonpb.ChatId) int {
		if queryOpts.Order == commonpb.QueryOptions_ASC {
			return bytes.Compare(a.GetValue(), b.GetValue())
		} else {
			return -1 * bytes.Compare(a.GetValue(), b.GetValue())
		}
	})

	if queryOpts.Token != nil {
		for i := range chatIDs {
			cmp := bytes.Compare(chatIDs[i].GetValue(), queryOpts.Token.GetValue())
			if queryOpts.Order == commonpb.QueryOptions_DESC {
				cmp *= -1
			}
			if cmp <= 0 {
				continue
			} else {
				chatIDs = chatIDs[i:]
				break
			}
		}
	}

	if queryOpts.Limit > 0 {
		chatIDs = chatIDs[:min(queryOpts.Limit, len(chatIDs))]
	}

	return chatIDs, nil
}

func (s *store) GetMembers(ctx context.Context, chatID *commonpb.ChatId) ([]*chat.Member, error) {
	encodedChatID := pg.Encode(chatID.Value)

	// TODO: Add pagination
	members, err := s.client.Member.FindMany(
		db.Member.ChatID.Equals(encodedChatID),
	).Exec(ctx)

	if err != nil {
		return nil, err
	}

	var pbMembers []*chat.Member
	for _, member := range members {
		decodedUserId, err := pg.Decode(member.UserID)
		if err != nil {
			return nil, err
		}

		pgMember := &chat.Member{
			UserID:   &commonpb.UserId{Value: decodedUserId},
			HasMuted: member.HasMuted,
			IsHost:   member.IsHost,
		}

		if addedByID, ok := member.AddedByID(); ok {
			decodedAddedBy, err := pg.Decode(addedByID)
			if err != nil {
				return nil, err
			}

			pgMember.AddedBy = &commonpb.UserId{Value: decodedAddedBy}
		}

		pbMembers = append(pbMembers, pgMember)
	}

	// TODO: Use the DB to sort the results or get rid of the sorting on the
	// memory store

	// (Can't sort on the DB side because the memory_store uses bytes.Compare and
	// the DB stores string values)
	// Fails on line 207 of store_test.go without this

	slices.SortFunc(pbMembers, func(a, b *chat.Member) int {
		return bytes.Compare(a.UserID.Value, b.UserID.Value)
	})

	return pbMembers, nil
}

func (s *store) GetMember(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (*chat.Member, error) {
	encodedChatID := pg.Encode(chatID.Value)
	encodedUserID := pg.Encode(userID.Value)

	member, err := s.client.Member.FindUnique(
		db.Member.ChatIDUserID(
			db.Member.ChatID.Equals(encodedChatID),
			db.Member.UserID.Equals(encodedUserID),
		),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) || member == nil {
		return nil, chat.ErrMemberNotFound
	}

	pgMember := &chat.Member{
		UserID:   &commonpb.UserId{Value: userID.Value},
		HasMuted: member.HasMuted,
		IsHost:   member.IsHost,
	}

	if addedByID, ok := member.AddedByID(); ok {
		decodedAddedBy, err := pg.Decode(addedByID)
		if err != nil {
			return nil, err
		}

		pgMember.AddedBy = &commonpb.UserId{Value: decodedAddedBy}
	}

	return pgMember, nil
}

func (s *store) IsMember(ctx context.Context, chatID *commonpb.ChatId, userID *commonpb.UserId) (bool, error) {
	encodedChatID := pg.Encode(chatID.Value)
	encodedUserID := pg.Encode(userID.Value)

	member, err := s.client.Member.FindUnique(
		db.Member.ChatIDUserID(
			db.Member.ChatID.Equals(encodedChatID),
			db.Member.UserID.Equals(encodedUserID),
		),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) || member == nil {
		return false, nil
	}

	return true, nil
}

func (s *store) CreateChat(ctx context.Context, md *chatpb.Metadata) (*chatpb.Metadata, error) {
	if md.ChatId == nil {
		return nil, errors.New("must provide chat id")
	}
	if md.RoomNumber != 0 {
		return nil, errors.New("cannot create chat with room number")
	}

	encodedChatID := pg.Encode(md.ChatId.Value)

	// Check if the chat already exists
	res, err := s.client.Chat.FindUnique(
		db.Chat.ID.Equals(encodedChatID),
	).Exec(ctx)

	if err == nil && res != nil {
		meta, err := fromModel(res)
		if err != nil {
			return nil, err
		}

		return meta, chat.ErrChatExists
	}
	if !errors.Is(err, db.ErrNotFound) && err != nil {
		return nil, err
	}

	// Find the next room number

	// TODO: This is not efficient, but it's fine for now?

	// Maybe an auto-incrementing field would be better, but thats not how the
	// in-memory store works. The memory store assumes that the room number can
	// be 0 multiple times indicating a chat without a room number. This would
	// fail db constraints.

	largest, err := s.client.Chat.FindFirst(
		db.Chat.Not(db.Chat.RoomNumber.IsNull()),
	).OrderBy(
		db.Chat.RoomNumber.Order(db.SortOrderDesc),
	).Exec(ctx)

	if !errors.Is(err, db.ErrNotFound) && err != nil {
		return nil, err
	}

	nextNumber := uint64(1)
	if largest != nil {
		if roomNumber, ok := largest.RoomNumber(); ok {
			nextNumber = uint64(roomNumber) + 1
		}
	}

	coverCharge := int(0)
	if md.CoverCharge != nil {
		coverCharge = int(md.CoverCharge.Quarks)
	}

	// Create the chat room
	res, err = s.client.Chat.CreateOne(
		db.Chat.ID.Set(encodedChatID),
		db.Chat.Title.Set(md.Title),
		db.Chat.RoomNumber.Set(int(nextNumber)),
		db.Chat.Type.Set(int(md.Type)),
		db.Chat.CoverCharge.Set(coverCharge),
	).Exec(ctx)

	if err != nil {
		return nil, err
	}

	// Add the owner as a member if provided
	if md.Owner != nil {
		encodedOwnerID := pg.Encode(md.Owner.Value)

		_, err = s.client.Member.CreateOne(
			db.Member.UserID.Set(encodedOwnerID),
			db.Member.Chat.Link(db.Chat.ID.Equals(encodedChatID)),
			db.Member.IsHost.Set(true),
		).Exec(ctx)

		if err != nil {
			return nil, err
		}

		return fromModelWithOwner(res, md.Owner)
	}

	return fromModel(res)
}

func (s *store) AddMember(ctx context.Context, chatID *commonpb.ChatId, member chat.Member) error {
	encodedChatID := pg.Encode(chatID.Value)
	encodedUserID := pg.Encode(member.UserID.Value)

	// Check if the chat exists
	res, err := s.client.Chat.FindUnique(
		db.Chat.ID.Equals(encodedChatID),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) || res == nil {
		return chat.ErrChatNotFound
	}

	// Check if the user is already a member
	_, err = s.client.Member.FindUnique(
		db.Member.ChatIDUserID(
			db.Member.ChatID.Equals(encodedChatID),
			db.Member.UserID.Equals(encodedUserID),
		),
	).Exec(ctx)
	if err == nil {
		return nil
	}

	// Create the member
	createArgs := []db.MemberSetParam{
		db.Member.HasMuted.Set(member.HasMuted),
		db.Member.IsHost.Set(member.IsHost),
	}

	// Add AddedBy parameter conditionally
	if member.AddedBy != nil {
		encodedAddedBy := pg.Encode(member.AddedBy.Value)
		createArgs = append(createArgs,
			db.Member.AddedByID.Set(encodedAddedBy),
		)
	}

	_, err = s.client.Member.CreateOne(
		db.Member.UserID.Set(encodedUserID),
		db.Member.Chat.Link(db.Chat.ID.Equals(encodedChatID)),
		createArgs...,
	).Exec(ctx)

	return err
}

func (s *store) RemoveMember(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) error {
	encodedChatID := pg.Encode(chatID.Value)
	encodedUserID := pg.Encode(member.Value)

	// Using FindMany().Delete() instead of FindUnique().Delete() because the
	// latter doesn't work with line 326 of store_test.go which expects no error
	// when deleting a non-existent member

	_, err := s.client.Member.FindMany(
		db.Member.ChatID.Equals(encodedChatID),
		db.Member.UserID.Equals(encodedUserID),
	).Delete().Exec(ctx)

	if errors.Is(err, db.ErrNotFound) {
		return chat.ErrMemberNotFound
	}

	return err
}

func (s *store) SetMuteState(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId, isMuted bool) error {
	encodedChatID := pg.Encode(chatID.Value)
	encodedUserID := pg.Encode(member.Value)

	_, err := s.client.Member.FindUnique(
		db.Member.ChatIDUserID(
			db.Member.ChatID.Equals(encodedChatID),
			db.Member.UserID.Equals(encodedUserID),
		),
	).Update(
		db.Member.HasMuted.Set(isMuted),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) {
		return chat.ErrMemberNotFound
	}

	return err
}

func (s *store) GetMuteState(ctx context.Context, chatID *commonpb.ChatId, member *commonpb.UserId) (bool, error) {
	encodedChatID := pg.Encode(chatID.Value)
	encodedUserID := pg.Encode(member.Value)

	res, err := s.client.Member.FindUnique(
		db.Member.ChatIDUserID(
			db.Member.ChatID.Equals(encodedChatID),
			db.Member.UserID.Equals(encodedUserID),
		),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) || res == nil {
		return false, chat.ErrMemberNotFound
	}

	return res.HasMuted, nil
}

func (s *store) SetCoverCharge(ctx context.Context, chatID *commonpb.ChatId, coverCharge *commonpb.PaymentAmount) error {
	encodedChatID := pg.Encode(chatID.Value)

	_, err := s.client.Chat.FindUnique(
		db.Chat.ID.Equals(encodedChatID),
	).Update(
		db.Chat.CoverCharge.Set(int(coverCharge.Quarks)),
	).Exec(ctx)

	if errors.Is(err, db.ErrNotFound) {
		return chat.ErrChatNotFound
	}

	return err
}
