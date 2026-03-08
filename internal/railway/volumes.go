package railway

import (
	"context"
	"fmt"
	"log/slog"
)

// CreateVolume creates a persistent volume attached to a service.
// Returns the volume ID on success.
func CreateVolume(ctx context.Context, client *Client, projectID, envID, serviceID, path, region string) (string, error) {
	slog.Debug("creating volume", "service_id", serviceID, "path", path, "region", region)
	input := VolumeCreateInput{
		ProjectId:     projectID,
		EnvironmentId: &envID,
		ServiceId:     &serviceID,
		MountPath:     path,
	}
	if region != "" {
		input.Region = &region
	}
	resp, err := VolumeCreate(ctx, client.gql(), input)
	if err != nil {
		return "", fmt.Errorf("creating volume at %q: %w", path, err)
	}
	return resp.VolumeCreate.Id, nil
}

// DeleteVolume deletes a persistent volume by ID.
func DeleteVolume(ctx context.Context, client *Client, volumeID string) error {
	slog.Debug("deleting volume", "volume_id", volumeID)
	_, err := VolumeDelete(ctx, client.gql(), volumeID)
	if err != nil {
		return fmt.Errorf("deleting volume %q: %w", volumeID, err)
	}
	return nil
}
