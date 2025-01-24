package tests

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	blobpb "github.com/code-payments/flipchat-protobuf-api/generated/go/blob/v1"
	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/flipchat-server/account"
	"github.com/code-payments/flipchat-server/auth"
	"github.com/code-payments/flipchat-server/blob"
	"github.com/code-payments/flipchat-server/model"
	"github.com/code-payments/flipchat-server/s3"
	"github.com/code-payments/flipchat-server/testutil"
)

func RunBlobServerTests(t *testing.T,
	accounts account.Store,
	blobStore blob.Store,
	s3Store s3.Store,
	teardown func()) {
	for _, tf := range []func(t *testing.T,
		accounts account.Store,
		blobStore blob.Store,
		s3Store s3.Store,
	){
		testServer,
	} {
		tf(t, accounts, blobStore, s3Store)
		teardown()
	}
}

func testServer(
	t *testing.T,
	accountStore account.Store,
	blobStore blob.Store,
	s3Store s3.Store,
) {
	log := zap.Must(zap.NewDevelopment())

	authz := account.NewAuthorizer(log, accountStore, auth.NewKeyPairAuthenticator())
	server := blob.NewServer(
		log,
		authz,
		blobStore,
		s3Store,
	)

	cc := testutil.RunGRPCServer(t, testutil.WithService(func(s *grpc.Server) {
		blobpb.RegisterBlobServiceServer(s, server)
	}))

	ctx := context.Background()
	client := blobpb.NewBlobServiceClient(cc)

	// Create a user and bind a key pair
	ownerKeyPair := model.MustGenerateKeyPair()
	userId := model.MustGenerateUserID()

	// Authorize the user
	_, _ = accountStore.Bind(ctx, userId, ownerKeyPair.Proto())

	var blobId *commonpb.BlobId

	t.Run("Upload_Success", func(t *testing.T) {
		rawData := []byte("hello world")

		req := &blobpb.UploadBlobRequest{
			OwnerId:  userId,
			BlobType: blobpb.BlobType_BLOB_TYPE_IMAGE,
			RawData:  rawData,
		}

		// Sign the request
		require.NoError(t, ownerKeyPair.Auth(req, &req.Auth))

		// Upload the blob
		resp, err := client.Upload(ctx, req)
		require.NoError(t, err, "Upload should succeed")
		require.NotNil(t, resp, "Upload response should not be nil")

		// Basic checks
		require.NotNil(t, resp.Blob)
		require.NotNil(t, resp.Blob.BlobId)
		require.NotEmpty(t, resp.Blob.BlobId.Value)
		require.Equal(t, userId.Value, resp.Blob.OwnerId.Value)
		require.Equal(t, blobpb.BlobType_BLOB_TYPE_IMAGE, resp.Blob.BlobType)
		require.NotEmpty(t, resp.Blob.S3Url)

		blobId = resp.Blob.BlobId // Store the generated ID for later tests

		// Check store
		storedBlob, err := blobStore.GetBlob(ctx, blobId)
		require.NoError(t, err, "blob should exist in the store after upload")
		require.NotNil(t, storedBlob, "stored blob should not be nil")

		// Compare store blob to returned proto blob
		requireProtoAndStoreBlobsEqual(t, resp.Blob, storedBlob)

		// Verify S3 content
		key, err := s3.ToS3Key(blobId.Value)
		require.NoError(t, err)
		data, err := s3Store.Download(ctx, key)
		require.NoError(t, err)
		require.Equal(t, rawData, data, "S3 content must match uploaded bytes")
	})

	t.Run("GetInfo_Success", func(t *testing.T) {
		require.NotNil(t, blobId, "Upload_Success must run first to set blobId")

		resp, err := client.GetInfo(ctx, &blobpb.GetBlobInfoRequest{
			BlobId: blobId,
		})
		require.NoError(t, err, "GetInfo should succeed for an existing blob")
		require.NotNil(t, resp)
		require.NotNil(t, resp.Blob)

		require.Equal(t, blobId.Value, resp.Blob.BlobId.Value)
		require.Equal(t, userId.Value, resp.Blob.OwnerId.Value)
		require.Equal(t, blobpb.BlobType_BLOB_TYPE_IMAGE, resp.Blob.BlobType)
		require.NotEmpty(t, resp.Blob.S3Url)
	})

	t.Run("Upload_Error_NoOwnerID", func(t *testing.T) {
		req := &blobpb.UploadBlobRequest{
			OwnerId:  nil, // Missing
			BlobType: blobpb.BlobType_BLOB_TYPE_IMAGE,
			RawData:  []byte("xyz"),
		}

		require.NoError(t, ownerKeyPair.Auth(req, &req.Auth))

		_, err := client.Upload(ctx, req)
		require.Error(t, err, "Upload must fail if owner_id is missing")
	})

	t.Run("Upload_Error_UnknownBlobType", func(t *testing.T) {
		req := &blobpb.UploadBlobRequest{
			OwnerId:  userId,
			BlobType: blobpb.BlobType_BLOB_TYPE_UNKNOWN, // invalid type
			RawData:  []byte("abc"),
		}

		require.NoError(t, ownerKeyPair.Auth(req, &req.Auth))

		_, err := client.Upload(ctx, req)
		require.Error(t, err, "Upload must fail if blob_type = UNKNOWN")
	})

	t.Run("Upload_Error_NoRawData", func(t *testing.T) {
		req := &blobpb.UploadBlobRequest{
			OwnerId:  userId,
			BlobType: blobpb.BlobType_BLOB_TYPE_VIDEO,
			RawData:  nil, // no raw_data
		}

		require.NoError(t, ownerKeyPair.Auth(req, &req.Auth))

		_, err := client.Upload(ctx, req)
		require.Error(t, err, "Upload must fail if raw_data is missing/empty")
	})

	t.Run("Upload_Error_InvalidSignature", func(t *testing.T) {
		req := &blobpb.UploadBlobRequest{
			OwnerId:  userId,
			BlobType: blobpb.BlobType_BLOB_TYPE_VIDEO,
			RawData:  []byte("xyz"),
			Auth:     nil, // empty Signature
		}

		_, err := client.Upload(ctx, req)
		require.Error(t, err, "Upload must fail if signature is missing")

		req = &blobpb.UploadBlobRequest{
			OwnerId:  userId,
			BlobType: blobpb.BlobType_BLOB_TYPE_VIDEO,
			RawData:  []byte("xyz"),
			Auth: &commonpb.Auth{
				Kind: &commonpb.Auth_KeyPair_{
					KeyPair: &commonpb.Auth_KeyPair{
						PubKey: &commonpb.PublicKey{
							Value: userId.Value,
						},
						Signature: &commonpb.Signature{
							Value: bytes.Repeat([]byte{0}, 64),
						}, // invalid Signature
					},
				},
			},
		}

		_, err = client.Upload(ctx, req)
		require.Error(t, err, "Upload must fail if signature is invalid")
	})

	t.Run("GetInfo_Error_NoBlobID", func(t *testing.T) {
		_, err := client.GetInfo(ctx, &blobpb.GetBlobInfoRequest{
			BlobId: nil, // missing
		})
		require.Error(t, err, "GetInfo must fail if blob_id is nil")
	})

	t.Run("GetInfo_Error_NotFound", func(t *testing.T) {
		randomBlobID := model.MustGenerateBlobID()
		_, err := client.GetInfo(ctx, &blobpb.GetBlobInfoRequest{
			BlobId: randomBlobID, // not stored
		})
		require.Error(t, err, "GetInfo must fail if the blob does not exist")
	})
}

func requireProtoAndStoreBlobsEqual(t *testing.T, protoBlob *blobpb.Blob, storeBlob *blob.Blob) {
	require.NotNil(t, protoBlob, "protoBlob must not be nil")
	require.NotNil(t, storeBlob, "storeBlob must not be nil")

	storeBlobAsProto, err := blob.ToProtoBlob(storeBlob)
	require.NoError(t, err, "converting store blob to proto should not fail")

	requireProtoBlobsEqual(t, protoBlob, storeBlobAsProto)
}

func requireProtoBlobsEqual(t *testing.T, expected, actual *blobpb.Blob) {
	require.NotNil(t, expected, "expected proto blob must not be nil")
	require.NotNil(t, actual, "actual proto blob must not be nil")

	require.Equal(t, expected.BlobId.GetValue(), actual.BlobId.GetValue(), "blob_id mismatch")
	require.Equal(t, expected.OwnerId.GetValue(), actual.OwnerId.GetValue(), "owner_id mismatch")
	require.Equal(t, expected.BlobType, actual.BlobType, "blob_type mismatch")
	require.Equal(t, expected.S3Url, actual.S3Url, "s3_url mismatch")

	// Check creation time with a small tolerance for nanos
	require.NotNil(t, expected.CreatedAt)
	require.NotNil(t, actual.CreatedAt)
	require.Equal(t, expected.CreatedAt.GetSeconds(), actual.CreatedAt.GetSeconds(), "CreatedAt seconds mismatch")
	require.InDelta(t,
		expected.CreatedAt.GetNanos(),
		actual.CreatedAt.GetNanos(),
		1_000_000, // up to microsecond tolerance
		"CreatedAt nanos mismatch",
	)
}
