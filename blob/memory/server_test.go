package memory

import (
	"testing"

	"github.com/code-payments/flipchat-server/blob/tests"

	account "github.com/code-payments/flipchat-server/account/memory"
	s3 "github.com/code-payments/flipchat-server/s3/memory"
)

func TestBlob_MemoryServer(t *testing.T) {
	testAccountStore := account.NewInMemory()
	testS3Store := s3.NewInMemory()
	testStore := NewInMemory()

	teardown := func() {
		testStore.(*store).reset()
	}
	tests.RunBlobServerTests(t, testAccountStore, testStore, testS3Store, teardown)
}
