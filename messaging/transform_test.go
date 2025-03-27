package messaging

import (
	"testing"

	"github.com/stretchr/testify/require"

	messagingpb "github.com/code-payments/flipchat-protobuf-api/generated/go/messaging/v1"
)

func TestRemoveEmojiVariationModifiersTransform(t *testing.T) {
	for _, tc := range []struct {
		in  string
		out string
	}{
		{
			in:  "",
			out: "",
		},
		{
			in:  "👍",
			out: "👍",
		},
		{
			in:  "\u2764\uFE0F",
			out: "\u2764",
		},
	} {
		in := &messagingpb.Content{
			Type: &messagingpb.Content_Reaction{
				Reaction: &messagingpb.ReactionContent{
					Emoji: tc.in,
				},
			},
		}
		out := RemoveEmojiVariationModifiersTransform(in)
		require.Equal(t, tc.out, out.GetReaction().Emoji)
	}
}
