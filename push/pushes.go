package push

import (
	"context"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	codekin "github.com/code-payments/code-server/pkg/kin"
)

var (
	kinAmountPrinter = message.NewPrinter(language.English)
)

func SendWeeklyAirdropPush(ctx context.Context, pusher Pusher, quarks uint64, users ...*commonpb.UserId) error {
	title := "Weekly Bonus Received"
	body := kinAmountPrinter.Sprintf("You received ⬢ %d Kin for being an active user", codekin.FromQuarks(quarks))
	return pusher.SendBasicPushes(ctx, title, body)
}
