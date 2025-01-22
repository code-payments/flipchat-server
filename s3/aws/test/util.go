package test

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/retry/backoff"
)

const (
	containerName     = "localstack"
	containerVersion  = "latest"
	containerAutoKill = 120 // seconds

	port      = 4566 // LocalStack Edge Port
	services  = "s3"
	accessKey = "test"
	secretKey = "test"
	region    = "us-east-1"
)

// StartLocalStackS3 starts a LocalStack container with S3 enabled.
// It returns the endpoint URL, a cleanup function, and any error encountered.
func StartLocalStackS3(pool *dockertest.Pool) (endpoint string, cleanup func(), err error) {
	// Run the LocalStack container with S3 service
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "localstack/localstack",
		Tag:        containerVersion,
		Env: []string{
			"SERVICES=" + services,
			"DEFAULT_REGION=" + region,
			"AWS_ACCESS_KEY_ID=" + accessKey,
			"AWS_SECRET_ACCESS_KEY=" + secretKey,
		},
		ExposedPorts: []string{fmt.Sprintf("%d/tcp", port)},
	}, func(config *docker.HostConfig) {
		// Enable AutoRemove and disable Restart
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})

	if err != nil {
		return "", nil, errors.Wrap(err, "could not start LocalStack container")
	}

	// Set a timeout to automatically kill the container
	resource.Expire(containerAutoKill)

	// Get the host and port to construct the endpoint URL
	hostAndPort := resource.GetHostPort(fmt.Sprintf("%d/tcp", port))
	endpoint = fmt.Sprintf("http://%s", hostAndPort)

	// Define the cleanup function
	cleanup = func() {
		if err := pool.Purge(resource); err != nil {
			fmt.Printf("Could not purge resource: %s\n", err)
		}
	}

	return endpoint, cleanup, nil
}

// WaitForS3Connection waits until the S3 endpoint is reachable.
// It retries the connection attempt with exponential backoff.
func WaitForS3Connection(endpoint string) error {
	_, err := retry.Retry(
		func() error {
			resp, err := http.Head(endpoint)
			if err != nil {
				return err
			}
			// LocalStack returns 404 for HEAD requests on S3 root
			// Consider 200, 404 as successful responses
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return nil
			}
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		},
		retry.Limit(50),
		retry.Backoff(backoff.Constant(500*time.Millisecond), 500*time.Second),
	)

	return err
}

// StartS3Mock initializes the LocalStack S3 mock and ensures it's ready.
// It returns the endpoint URL, a cleanup function, and any error encountered.
func StartS3Mock() (endpoint string, cleanup func(), err error) {
	// Initialize a new Dockertest pool
	pool, err := dockertest.NewPool("")
	if err != nil {
		return "", nil, errors.Wrap(err, "could not connect to docker")
	}

	// Start the LocalStack container
	endpoint, cleanup, err = StartLocalStackS3(pool)
	if err != nil {
		return "", nil, err
	}

	// Wait for the S3 service to be ready
	err = WaitForS3Connection(endpoint)
	if err != nil {
		cleanup()
		return "", nil, err
	}

	return endpoint, cleanup, nil
}
