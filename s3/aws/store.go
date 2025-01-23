package aws

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/s3iface"

	fc_s3 "github.com/code-payments/flipchat-server/s3"
)

type store struct {
	client s3iface.ClientAPI
	bucket string
}

func NewAWSStore(client s3iface.ClientAPI, bucket string) fc_s3.Store {
	return &store{
		client: client,
		bucket: bucket,
	}
}

// Upload uploads the data to S3 at the specified key.
// Returns the URL path to the uploaded data.
func (s *store) Upload(ctx context.Context, key string, data []byte) (string, error) {
	if key == "" {
		return "", fmt.Errorf("key cannot be empty")
	}
	if data == nil {
		return "", fmt.Errorf("data cannot be nil")
	}

	req := s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	}

	// Upload the object to S3
	_, err := s.client.PutObjectRequest(&req).Send(ctx)
	if err != nil {
		return "", err
	}

	return fc_s3.GenerateS3URLPathForByteId(key, s.bucket)
}

// Download retrieves the data from S3 at the specified key.
func (a *store) Download(ctx context.Context, key string) ([]byte, error) {
	if key == "" {
		return nil, fmt.Errorf("key cannot be empty")
	}

	req := s3.GetObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(key),
	}

	// Download the object from s3iface
	resp, err := a.client.GetObjectRequest(&req).Send(ctx)
	if err != nil {
		return nil, err
	}

	// Read the data from the response
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}
