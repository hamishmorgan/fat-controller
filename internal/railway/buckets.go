package railway

import (
	"context"
	"fmt"
	"log/slog"
)

// CreateBucket creates a new bucket in a project.
// Returns the bucket ID on success.
func CreateBucket(ctx context.Context, client *Client, projectID, name string) (string, error) {
	slog.Debug("creating bucket", "project_id", projectID, "name", name)
	input := BucketCreateInput{
		ProjectId: projectID,
		Name:      &name,
	}
	resp, err := BucketCreate(ctx, client.GQL(), input)
	if err != nil {
		return "", fmt.Errorf("creating bucket %q: %w", name, err)
	}
	return resp.BucketCreate.Id, nil
}

// UpdateBucket updates a bucket's name.
func UpdateBucket(ctx context.Context, client *Client, id, name string) error {
	slog.Debug("updating bucket", "id", id, "name", name)
	input := BucketUpdateInput{
		Name: name,
	}
	_, err := BucketUpdate(ctx, client.GQL(), id, input)
	if err != nil {
		return fmt.Errorf("updating bucket %q: %w", id, err)
	}
	return nil
}

// BucketCredentials contains S3-compatible credentials for a bucket.
type BucketCredentials struct {
	AccessKeyId     string
	SecretAccessKey string
	BucketName      string
	Endpoint        string
	Region          string
	UrlStyle        string
}

// GetBucketCredentials retrieves S3-compatible credentials for a bucket.
func GetBucketCredentials(ctx context.Context, client *Client, bucketID, environmentID, projectID string) ([]BucketCredentials, error) {
	slog.Debug("getting bucket credentials", "bucket_id", bucketID, "environment_id", environmentID)
	resp, err := BucketS3Credentials(ctx, client.GQL(), bucketID, environmentID, projectID)
	if err != nil {
		return nil, fmt.Errorf("getting bucket credentials for %q: %w", bucketID, err)
	}
	creds := make([]BucketCredentials, len(resp.BucketS3Credentials))
	for i, c := range resp.BucketS3Credentials {
		creds[i] = BucketCredentials{
			AccessKeyId:     c.AccessKeyId,
			SecretAccessKey: c.SecretAccessKey,
			BucketName:      c.BucketName,
			Endpoint:        c.Endpoint,
			Region:          c.Region,
			UrlStyle:        c.UrlStyle,
		}
	}
	return creds, nil
}
