package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatpb "github.com/code-payments/flipchat-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/model"
)

func createRooms(t *testing.T, chatStore chat.Store, count int) []*chatpb.Metadata {
	rooms := make([]*chatpb.Metadata, count)
	for i := 0; i < count; i++ {
		chatID := model.MustGenerateChatID()
		data := &chatpb.Metadata{
			ChatId:       chatID,
			Type:         chatpb.Metadata_GROUP,
			DisplayName:  fmt.Sprintf("Room %d", i),
			Owner:        model.MustGenerateUserID(),
			MessagingFee: &commonpb.PaymentAmount{Quarks: 1},
			NumUnread:    0,
			LastActivity: &timestamppb.Timestamp{Seconds: time.Now().Unix()},
			OpenStatus:   &chatpb.OpenStatus{IsCurrentlyOpen: true},
		}

		_, err := chatStore.CreateChat(context.Background(), data)
		require.NoError(t, err)

		rooms[i] = data
	}

	return rooms
}
