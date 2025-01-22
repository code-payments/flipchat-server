package aws

import (
	"testing"

	"github.com/stretchr/testify/require"

	awstest "github.com/code-payments/flipchat-server/s3/aws/test"
	"github.com/code-payments/flipchat-server/s3/tests"
)

func TestAWSStore(t *testing.T) {
	// Start the S3 mock
	endpoint, cleanup, err := awstest.StartS3Mock()
	require.NoError(t, err, "Failed to start S3 mock")
	defer cleanup()

	// Initialize the AWSStore with the mock endpoint
	store, err := NewAWSStoreWithEndpoint(endpoint)
	require.NoError(t, err, "Failed to initialize AWSStore with mock endpoint")

	// Run the store tests
	tests.RunStoreTests(t, store, func() {
		// No additional teardown needed as cleanup is handled by the defer statement
	})
}
