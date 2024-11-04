package messaging

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
)

func MustGenerateMessageID() *messagingpb.MessageId {
	id, err := GenerateMessageID()
	if err != nil {
		panic(fmt.Sprintf("failed to generate message id: %v", err))
	}

	return id
}

func GenerateMessageID() (*messagingpb.MessageId, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &messagingpb.MessageId{Value: id[:]}, nil
}

func MustGenerateMessageIDFromTime(t time.Time) *messagingpb.MessageId {
	id, err := GenerateMessageIDFromTime(t)
	if err != nil {
		panic(fmt.Sprintf("failed to generate message id: %v", err))
	}

	return id
}

func GenerateMessageIDFromTime(t time.Time) (*messagingpb.MessageId, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	// Convert timestamp to milliseconds since Unix epoch
	millis := t.UnixNano() / int64(time.Millisecond)

	// Populate the first 6 bytes with the timestamp (42 bits for timestamp)
	id[0] = byte((millis >> 40) & 0xff)
	id[1] = byte((millis >> 32) & 0xff)
	id[2] = byte((millis >> 24) & 0xff)
	id[3] = byte((millis >> 16) & 0xff)
	id[4] = byte((millis >> 8) & 0xff)
	id[5] = byte(millis & 0xff)

	return &messagingpb.MessageId{Value: id[:]}, nil
}
