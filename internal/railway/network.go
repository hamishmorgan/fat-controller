package railway

import (
	"context"
	"fmt"
	"log/slog"
)

// EnablePrivateNetwork creates or gets a private network endpoint for a service.
// This creates the private network if needed, then attaches the service to it.
// Returns the DNS name of the endpoint on success.
func EnablePrivateNetwork(ctx context.Context, client *Client, envID, serviceID string) (string, error) {
	slog.Debug("enabling private network", "service_id", serviceID)
	input := PrivateNetworkEndpointCreateOrGetInput{
		EnvironmentId:    envID,
		PrivateNetworkId: "",
		ServiceId:        serviceID,
		ServiceName:      "",
		Tags:             []string{},
	}
	resp, err := PrivateNetworkEndpointCreateOrGet(ctx, client.gql(), input)
	if err != nil {
		return "", fmt.Errorf("enabling private network for service %q: %w", serviceID, err)
	}
	return resp.PrivateNetworkEndpointCreateOrGet.PrivateNetworkEndpointFields.DnsName, nil
}

// DisablePrivateNetworkEndpoint deletes a private network endpoint by ID.
func DisablePrivateNetworkEndpoint(ctx context.Context, client *Client, id string) error {
	slog.Debug("disabling private network endpoint", "id", id)
	_, err := PrivateNetworkEndpointDelete(ctx, client.gql(), id)
	if err != nil {
		return fmt.Errorf("disabling private network endpoint %q: %w", id, err)
	}
	return nil
}
