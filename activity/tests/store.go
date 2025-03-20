package tests

import (
	"testing"

	"github.com/code-payments/flipchat-server/activity"
)

func RunStoreTests(t *testing.T, s activity.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s activity.Store){
		testActivityStore_HappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testActivityStore_HappyPath(t *testing.T, store activity.Store) {
}
