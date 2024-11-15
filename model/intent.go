package model

import (
	"fmt"

	"github.com/mr-tron/base58"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

func MustGenerateIntentID() *commonpb.IntentId {
	id, err := GenerateIntentID()
	if err != nil {
		panic(fmt.Sprintf("failed to generate intent id: %v", err))
	}

	return id
}

func GenerateIntentID() (*commonpb.IntentId, error) {
	kp, err := GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	return &commonpb.IntentId{Value: kp.Public()}, nil
}

func IntentIDString(intent *commonpb.IntentId) string {
	return base58.Encode(intent.Value)
}
