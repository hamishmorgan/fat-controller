package railway

import (
	"context"
	"fmt"
	"log/slog"
)

// BucketInfo holds the name and ID of a Railway bucket.
type BucketInfo struct {
	ID   string `json:"id" toml:"id"`
	Name string `json:"name" toml:"name"`
}

// bucketInfoFromSummary converts a generated BucketSummaryFields
// fragment into the public BucketInfo type.
func bucketInfoFromSummary(b *BucketSummaryFields) BucketInfo {
	return BucketInfo{ID: b.Id, Name: b.Name}
}

// ListBuckets returns the name/ID pairs for all buckets in a project.
func ListBuckets(ctx context.Context, client *Client, projectID string) ([]BucketInfo, error) {
	resp, err := ProjectBuckets(ctx, client.gql(), projectID)
	if err != nil {
		return nil, err
	}
	buckets := make([]BucketInfo, len(resp.Project.Buckets.Edges))
	for i, edge := range resp.Project.Buckets.Edges {
		buckets[i] = bucketInfoFromSummary(&edge.Node.BucketSummaryFields)
	}
	return buckets, nil
}

// CreateBucket creates a new bucket in a project.
// Returns the bucket ID on success.
func CreateBucket(ctx context.Context, client *Client, projectID, name string) (string, error) {
	slog.Debug("creating bucket", "project_id", projectID, "name", name)
	input := BucketCreateInput{
		ProjectId: projectID,
		Name:      &name,
	}
	resp, err := BucketCreate(ctx, client.gql(), input)
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
	_, err := BucketUpdate(ctx, client.gql(), id, input)
	if err != nil {
		return fmt.Errorf("updating bucket %q: %w", id, err)
	}
	return nil
}

// GetBucketCredentials retrieves S3-compatible credentials for a bucket.
// Returns the generated BucketCredentialFields fragment type directly.
func GetBucketCredentials(ctx context.Context, client *Client, bucketID, environmentID, projectID string) ([]BucketCredentialFields, error) {
	slog.Debug("getting bucket credentials", "bucket_id", bucketID, "environment_id", environmentID)
	resp, err := BucketS3Credentials(ctx, client.gql(), bucketID, environmentID, projectID)
	if err != nil {
		return nil, fmt.Errorf("getting bucket credentials for %q: %w", bucketID, err)
	}
	return resp.BucketS3Credentials, nil
}
