package messaging

import (
	"unicode"

	"google.golang.org/protobuf/proto"

	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
)

var (
	AllTransforms = []Transform{
		RemoveEmojiVariationModifiersTransform,
	}
)

type Transform func(*messagingpb.Content) *messagingpb.Content

func RemoveEmojiVariationModifiersTransform(content *messagingpb.Content) *messagingpb.Content {
	if content.GetReaction() == nil {
		return content
	}

	cloned := proto.Clone(content).(*messagingpb.Content)

	var transformed []rune
	for _, r := range cloned.GetReaction().Emoji {
		if !unicode.In(r, unicode.Variation_Selector) {
			transformed = append(transformed, r)
		}
	}
	cloned.GetReaction().Emoji = string(transformed)

	return cloned
}

func ApplyTransforms(content *messagingpb.Content) *messagingpb.Content {
	for _, transform := range AllTransforms {
		content = transform(content)
	}
	return content
}
