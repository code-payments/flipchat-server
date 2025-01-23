package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	blobpb "github.com/code-payments/flipchat-protobuf-api/generated/go/blob/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/blob"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/s3"
	"github.com/code-payments/flipchat-server/testutil"
)

func RunBlobServerTests(t *testing.T, blobStore blob.Store, s3Store s3.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, blobStore blob.Store, s3Store s3.Store){
		testServer,
	} {
		tf(t, blobStore, s3Store)
		teardown()
	}
}

func testServer(t *testing.T, blobStore blob.Store, s3Store s3.Store) {
	server := blob.NewServer(
		zap.Must(zap.NewDevelopment()),
		blobStore,
		s3Store,
	)

	cc := testutil.RunGRPCServer(t, testutil.WithService(func(s *grpc.Server) {
		blobpb.RegisterBlobServiceServer(s, server)
	}))

	ctx := context.Background()
	client := blobpb.NewBlobServiceClient(cc)

	userId := model.MustGenerateUserID()
	var blobId *commonpb.BlobId

	t.Run("Upload", func(t *testing.T) {
		// Upload the blob to the server
		rawData := []byte("hello world")
		resp, err := client.Upload(ctx, &blobpb.UploadBlobRequest{
			OwnerId:  userId,
			BlobType: blobpb.BlobType_BLOB_TYPE_IMAGE,
			RawData:  rawData,
		})

		// Check the blob response
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.Blob.BlobId)
		require.Equal(t, userId.Value, resp.Blob.OwnerId.Value)
		require.Equal(t, blobpb.BlobType_BLOB_TYPE_IMAGE, resp.Blob.BlobType)
		require.NotEmpty(t, resp.Blob.S3Url)

		// Check that the blob was stored in the store
		storedBlob, err := blobStore.GetBlob(ctx, resp.Blob.BlobId)
		require.NoError(t, err)
		require.NotNil(t, storedBlob)
		require.Equal(t, resp.Blob.BlobId.Value, storedBlob.ID.Value)
		require.Equal(t, userId.Value, storedBlob.UserID.Value)
		require.Equal(t, blob.BlobTypeImage, storedBlob.Type)
		require.NotEmpty(t, storedBlob.S3URL)

		// Check that the blob was stored in the S3 store
		key, err := s3.ToS3Key(resp.Blob.BlobId.Value)
		require.NoError(t, err)
		data, err := s3Store.Download(ctx, key)
		require.NoError(t, err)
		require.Equal(t, rawData, data)

		blobId = resp.Blob.BlobId
	})

	t.Run("GetInfo", func(t *testing.T) {
		// Get the blob info back from the testServer
		resp, err := client.GetInfo(ctx, &blobpb.GetBlobInfoRequest{
			BlobId: blobId,
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, blobId.Value, resp.Blob.BlobId.Value)
		require.Equal(t, userId.Value, resp.Blob.OwnerId.Value)
		require.Equal(t, blobpb.BlobType_BLOB_TYPE_IMAGE, resp.Blob.BlobType)
		require.NotEmpty(t, resp.Blob.S3Url)
	})
}
