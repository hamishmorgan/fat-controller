package railway

import (
	"context"
	"fmt"
	"log/slog"
)

// CreateCustomDomain creates a custom domain for a service.
// Returns the domain string (e.g. "example.com") on success.
func CreateCustomDomain(ctx context.Context, client *Client, projectID, envID, serviceID, domain string, port int) (string, error) {
	slog.Debug("creating custom domain", "service_id", serviceID, "domain", domain, "port", port)
	input := CustomDomainCreateInput{
		ProjectId:     projectID,
		EnvironmentId: envID,
		ServiceId:     serviceID,
		Domain:        domain,
		TargetPort:    &port,
	}
	resp, err := CustomDomainCreate(ctx, client.GQL(), input)
	if err != nil {
		return "", fmt.Errorf("creating custom domain %q: %w", domain, err)
	}
	return resp.CustomDomainCreate.Id, nil
}

// DeleteCustomDomain deletes a custom domain by ID.
func DeleteCustomDomain(ctx context.Context, client *Client, id string) error {
	slog.Debug("deleting custom domain", "id", id)
	_, err := CustomDomainDelete(ctx, client.GQL(), id)
	if err != nil {
		return fmt.Errorf("deleting custom domain %q: %w", id, err)
	}
	return nil
}

// CreateServiceDomain creates a Railway-managed service domain (*.up.railway.app).
// Returns the generated domain string on success.
func CreateServiceDomain(ctx context.Context, client *Client, envID, serviceID string, port int) (string, error) {
	slog.Debug("creating service domain", "service_id", serviceID, "port", port)
	input := ServiceDomainCreateInput{
		EnvironmentId: envID,
		ServiceId:     serviceID,
		TargetPort:    &port,
	}
	resp, err := ServiceDomainCreate(ctx, client.GQL(), input)
	if err != nil {
		return "", fmt.Errorf("creating service domain: %w", err)
	}
	return resp.ServiceDomainCreate.Domain, nil
}

// DeleteServiceDomain deletes a service domain by ID.
func DeleteServiceDomain(ctx context.Context, client *Client, id string) error {
	slog.Debug("deleting service domain", "id", id)
	_, err := ServiceDomainDelete(ctx, client.GQL(), id)
	if err != nil {
		return fmt.Errorf("deleting service domain %q: %w", id, err)
	}
	return nil
}
