package railway

import (
	"context"
	"fmt"
	"log/slog"
)

// CreateTCPProxy creates a TCP proxy for a service on the given application port.
// Returns the proxy ID on success.
func CreateTCPProxy(ctx context.Context, client *Client, envID, serviceID string, port int) (string, error) {
	slog.Debug("creating TCP proxy", "service_id", serviceID, "port", port)
	input := TCPProxyCreateInput{
		ApplicationPort: port,
		EnvironmentId:   envID,
		ServiceId:       serviceID,
	}
	resp, err := TcpProxyCreate(ctx, client.GQL(), input)
	if err != nil {
		return "", fmt.Errorf("creating TCP proxy on port %d: %w", port, err)
	}
	return resp.TcpProxyCreate.Id, nil
}

// DeleteTCPProxy deletes a TCP proxy by ID.
func DeleteTCPProxy(ctx context.Context, client *Client, id string) error {
	slog.Debug("deleting TCP proxy", "id", id)
	_, err := TcpProxyDelete(ctx, client.GQL(), id)
	if err != nil {
		return fmt.Errorf("deleting TCP proxy %q: %w", id, err)
	}
	return nil
}
