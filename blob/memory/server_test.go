package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/blob/tests"
	s3 "github.com/code-payments/flipchat-server/s3/memory"
)

func TestBlob_MemoryServer(t *testing.T) {
	testS3Store := s3.NewInMemory()
	testStore := NewInMemory()
	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunBlobServerTests(t, testStore, testS3Store, teardown)
}
