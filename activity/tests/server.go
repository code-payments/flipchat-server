package tests

import (
	"testing"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/activity"
	"github.com/code-payments/flipchat-server/chat"
	"github.com/code-payments/flipchat-server/profile"
)

func RunServerTests(t *testing.T, accounts account.Store, activityFeeds activity.Store, chats chat.Store, profiles profile.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, accounts account.Store, activityFeeds activity.Store, chats chat.Store, profiles profile.Store){
		testActivityServer_HappyPath,
	} {
		tf(t, accounts, activityFeeds, chats, profiles)
		teardown()
	}
}

func testActivityServer_HappyPath(t *testing.T, accounts account.Store, activityFeeds activity.Store, chats chat.Store, profiles profile.Store) {
}
