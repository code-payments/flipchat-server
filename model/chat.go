package model

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

func GenerateChatID() (*commonpb.ChatId, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	return &commonpb.ChatId{Value: id[:]}, err
}

func MustGenerateChatID() *commonpb.ChatId {
	id, err := GenerateChatID()
	if err != nil {
		panic(fmt.Sprintf("failed to generate chat id: %v", err))
	}

	return id
}

func GenerateTwoWayChatID(a, b *commonpb.UserId) (*commonpb.ChatId, error) {
	var users []*commonpb.UserId
	if bytes.Compare(a.Value, b.Value) <= 0 {
		users = []*commonpb.UserId{a, b}
	} else {
		users = []*commonpb.UserId{b, a}
	}

	buf := make([]byte, len(users[0].Value)+len(users[1].Value))
	copy(buf, users[0].Value)
	copy(buf[len(users[0].Value):], users[1].Value)

	h := sha256.Sum256(buf)
	return &commonpb.ChatId{Value: h[:]}, nil
}

func MustGenerateTwoWayChatID(a, b *commonpb.UserId) *commonpb.ChatId {
	id, err := GenerateTwoWayChatID(a, b)
	if err != nil {
		panic(fmt.Sprintf("failed to generate two way chat id: %v", err))
	}

	return id
}
