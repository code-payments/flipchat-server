package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/preview/tests"
)

func TestPreview_MemoryStore(t *testing.T) {
	store := NewInMemory()

	teardown := func() {
		store.(*memoryStore).reset()
	}
	tests.RunStoreTests(t, store, teardown)
}
