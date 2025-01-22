// File: s3/aws/store.go
package aws

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"

	"github.com/aws/aws-sdk-go-v2/aws"
	aws_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/code-payments/flipchat-server/s3"
)

// AWSStore is the AWS S3 implementation of the s3.Store interface.
type AWSStore struct {
	client *aws_s3.Client
	bucket string
}

// NewAWSStore initializes and returns a new AWSStore.
// It manually sets up the AWS configuration without using config.LoadDefaultConfig or aws/credentials.
func NewAWSStore(endpoint, region, bucket string) (*AWSStore, error) {
	// Define AWS credentials manually
	// Replace "accesskey" and "secretkey" with your actual credentials or use environment variables
	creds := aws.NewStaticCredentialsProvider("accesskey", "secretkey", "")

	// Define AWS configuration
	cfg := aws.Config{
		Region:      region,
		Credentials: creds,
	}

	// If a custom endpoint is provided (e.g., LocalStack), set it
	if endpoint != "" {
		cfg.Endpoint = endpoint
	}

	// Create an S3 client
	client := aws_s3.New(&cfg)

	return &AWSStore{
		client: client,
		bucket: bucket,
	}, nil
}

// Upload uploads the data to S3 at the specified key.
func (a *AWSStore) Upload(ctx context.Context, key string, data []byte) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}

	// Prepare the PutObject input
	input := &aws_s3.PutObjectInput{
		Bucket:        aws.String(a.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	}

	// Upload the object to S3
	_, err := a.client.PutObject(ctx, input)
	if err != nil {
		return errors.Wrap(err, "failed to upload object to S3")
	}

	log.Infof("Uploaded data to s3://%s/%s", a.bucket, key)
	return nil
}

// Download retrieves the data from S3 at the specified key.
func (a *AWSStore) Download(ctx context.Context, key string) ([]byte, error) {
	if key == "" {
		return nil, fmt.Errorf("key cannot be empty")
	}

	// Prepare the GetObject input
	input := &aws_s3.GetObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(key),
	}

	// Retrieve the object from S3
	output, err := a.client.GetObject(ctx, input)
	if err != nil {
		// Check if the error is due to the object not existing
		if aerr, ok := err.(*s3.NoSuchKey); ok && aerr != nil {
			return nil, s3.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to download object from S3")
	}
	defer output.Body.Close()

	// Read the object's data
	data, err := ioutil.ReadAll(output.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read object data")
	}

	log.Infof("Downloaded data from s3://%s/%s", a.bucket, key)
	return data, nil
}
