package model

import (
	"fmt"

	"github.com/google/uuid"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

func GeneratePreviewID() (*commonpb.PreviewId, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	return &commonpb.PreviewId{Value: id[:]}, err
}

func MustGeneratePreviewID() *commonpb.PreviewId {
	id, err := GeneratePreviewID()
	if err != nil {
		panic(fmt.Sprintf("failed to generate preview id: %v", err))
	}

	return id
}
