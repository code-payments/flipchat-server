package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/s3/tests"
)

func TestS3_MemoryStore(t *testing.T) {
	testStore := NewInMemory()
	teardown := func() {
		// testStore.(*memory).reset()
	}
	tests.RunStoreTests(t, testStore, teardown)
}
