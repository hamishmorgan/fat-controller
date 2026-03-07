package railway

import (
	"context"
	"fmt"
	"log/slog"
)

// CreateEgressGateway creates an egress gateway association for a service.
// Returns the assigned IPv4 address on success.
func CreateEgressGateway(ctx context.Context, client *Client, envID, serviceID, region string) (string, error) {
	slog.Debug("creating egress gateway", "service_id", serviceID, "region", region)
	input := EgressGatewayCreateInput{
		EnvironmentId: envID,
		ServiceId:     serviceID,
		Region:        &region,
	}
	resp, err := EgressGatewayAssociationCreate(ctx, client.GQL(), input)
	if err != nil {
		return "", fmt.Errorf("creating egress gateway: %w", err)
	}
	if len(resp.EgressGatewayAssociationCreate) == 0 {
		return "", fmt.Errorf("creating egress gateway: no gateways returned")
	}
	return resp.EgressGatewayAssociationCreate[0].Ipv4, nil
}

// ClearEgressGateways removes all egress gateway associations for a service.
func ClearEgressGateways(ctx context.Context, client *Client, envID, serviceID string) error {
	slog.Debug("clearing egress gateways", "service_id", serviceID)
	input := EgressGatewayServiceTargetInput{
		EnvironmentId: envID,
		ServiceId:     serviceID,
	}
	_, err := EgressGatewayAssociationsClear(ctx, client.GQL(), input)
	if err != nil {
		return fmt.Errorf("clearing egress gateways for service %q: %w", serviceID, err)
	}
	return nil
}
