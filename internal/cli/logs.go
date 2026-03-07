package cli

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// LogsCmd implements the `logs` command.
type LogsCmd struct {
	EnvironmentFlags `kong:"embed"`
	Services         []string `arg:"" optional:"" help:"Services to show logs for."`
	Build            bool     `help:"Show build logs." short:"b"`
	Lines            *int     `help:"Number of lines to fetch." short:"n"`
	Since            string   `help:"Start time: relative (5m, 2h) or ISO 8601."`
	Until            string   `help:"End time: relative or ISO 8601."`
	Filter           string   `help:"Filter expression." short:"f"`
}

func (c *LogsCmd) Run(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}

	projID, envID, err := railway.ResolveProjectEnvironment(ctx, client, c.Workspace, c.Project, c.Environment)
	if err != nil {
		return err
	}

	// If no services specified and no build flag, use environment logs.
	if len(c.Services) == 0 && !c.Build {
		var filter *string
		if c.Filter != "" {
			filter = &c.Filter
		}
		entries, err := railway.GetEnvironmentLogs(ctx, client, envID, c.Lines, filter)
		if err != nil {
			return err
		}
		for _, e := range entries {
			_, _ = fmt.Fprintf(os.Stdout, "%s %s %s\n", e.Timestamp, severity(e.Severity), e.Message)
		}
		return nil
	}

	// Resolve service targets.
	targets, err := resolveServiceTargets(ctx, client, projID, envID, c.Services)
	if err != nil {
		return err
	}

	// Parse time bounds.
	var startDate, endDate *time.Time
	if c.Since != "" {
		t, err := parseTimeArg(c.Since)
		if err != nil {
			return fmt.Errorf("invalid --since: %w", err)
		}
		startDate = &t
	}
	if c.Until != "" {
		t, err := parseTimeArg(c.Until)
		if err != nil {
			return fmt.Errorf("invalid --until: %w", err)
		}
		endDate = &t
	}

	var filter *string
	if c.Filter != "" {
		filter = &c.Filter
	}

	for _, svc := range targets {
		// Find latest deployment.
		deployments, _, err := railway.ListDeployments(ctx, client, envID, svc.ID, 1, nil)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to list deployments for %s: %v\n", svc.Name, err)
			continue
		}
		if len(deployments) == 0 {
			_, _ = fmt.Fprintf(os.Stderr, "no deployments found for %s\n", svc.Name)
			continue
		}

		var entries []railway.LogEntry
		if c.Build {
			entries, err = railway.GetBuildLogs(ctx, client, deployments[0].ID, c.Lines, startDate, endDate, filter)
		} else {
			entries, err = railway.GetDeploymentLogs(ctx, client, deployments[0].ID, c.Lines, startDate, endDate, filter)
		}
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to fetch logs for %s: %v\n", svc.Name, err)
			continue
		}

		for _, e := range entries {
			_, _ = fmt.Fprintf(os.Stdout, "[%s] %s %s %s\n", svc.Name, e.Timestamp, severity(e.Severity), e.Message)
		}
	}
	return nil
}

// severity returns the severity string or an empty string if nil.
func severity(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseTimeArg parses a relative duration (5m, 2h, 1d) or ISO 8601 timestamp.
func parseTimeArg(s string) (time.Time, error) {
	// Try relative duration first.
	if d, err := parseRelativeDuration(s); err == nil {
		return time.Now().Add(-d), nil
	}
	// Try ISO 8601.
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("expected relative duration (5m, 2h, 1d) or RFC3339 timestamp, got %q", s)
	}
	return t, nil
}

// parseRelativeDuration parses strings like "5m", "2h", "1d" to time.Duration.
func parseRelativeDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("too short")
	}
	suffix := s[len(s)-1]
	numStr := s[:len(s)-1]
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, err
	}
	switch suffix {
	case 's':
		return time.Duration(n) * time.Second, nil
	case 'm':
		return time.Duration(n) * time.Minute, nil
	case 'h':
		return time.Duration(n) * time.Hour, nil
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown suffix: %c", suffix)
	}
}
