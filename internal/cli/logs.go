package cli

import (
	"context"
	"fmt"
	"io"
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
	Deploy           bool     `help:"Show deploy logs (default when services specified)." short:"d"`
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

	// --deploy is the explicit form of the default (deploy logs). If both
	// --build and --deploy are set, --build wins since build logs are a
	// subset of the deployment lifecycle.
	showBuild := c.Build && !c.Deploy

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
		return RunLogsEnvironment(globals, envID, c.Lines, entries, os.Stdout)
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

	return RunLogsServices(ctx, globals, envID, targets, showBuild, c.Lines,
		func(ctx context.Context, environmentID, serviceID string) ([]railway.DeploymentInfo, error) {
			d, _, err := railway.ListDeployments(ctx, client, environmentID, serviceID, 1, nil)
			return d, err
		},
		func(ctx context.Context, deploymentID string, isBuild bool) ([]railway.LogEntry, error) {
			if isBuild {
				return railway.GetBuildLogs(ctx, client, deploymentID, c.Lines, startDate, endDate, filter)
			}
			return railway.GetDeploymentLogs(ctx, client, deploymentID, c.Lines, startDate, endDate, filter)
		},
		os.Stdout, os.Stderr,
	)
}

type LogEntryOut struct {
	Timestamp string `json:"timestamp" toml:"timestamp"`
	Severity  string `json:"severity,omitempty" toml:"severity"`
	Message   string `json:"message" toml:"message"`
}

type LogsResult struct {
	Scope        string        `json:"scope" toml:"scope"`
	Service      string        `json:"service,omitempty" toml:"service"`
	ServiceID    string        `json:"service_id,omitempty" toml:"service_id"`
	Lines        *int          `json:"lines,omitempty" toml:"lines"`
	Build        bool          `json:"build" toml:"build"`
	DeploymentID string        `json:"deployment_id,omitempty" toml:"deployment_id"`
	Entries      []LogEntryOut `json:"entries,omitempty" toml:"entries"`
	Error        string        `json:"error,omitempty" toml:"error"`
}

type LogsOutput struct {
	EnvironmentID string       `json:"environment_id" toml:"environment_id"`
	Results       []LogsResult `json:"results" toml:"results"`
}

func RunLogsEnvironment(globals *Globals, environmentID string, lines *int, entries []railway.LogEntry, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}
	if isStructuredOutput(globals) {
		payload := LogsOutput{EnvironmentID: environmentID, Results: []LogsResult{{Scope: "environment", Build: false, Lines: lines, Entries: make([]LogEntryOut, 0, len(entries))}}}
		for _, e := range entries {
			payload.Results[0].Entries = append(payload.Results[0].Entries, LogEntryOut{Timestamp: e.Timestamp, Severity: severity(e.Severity), Message: e.Message})
		}
		return writeStructured(out, globals.Output, payload)
	}
	for _, e := range entries {
		_, _ = fmt.Fprintf(out, "%s %s %s\n", e.Timestamp, severity(e.Severity), e.Message)
	}
	return nil
}

func RunLogsServices(
	ctx context.Context,
	globals *Globals,
	environmentID string,
	targets []serviceTarget,
	build bool,
	lines *int,
	listLatest func(ctx context.Context, environmentID, serviceID string) ([]railway.DeploymentInfo, error),
	fetchLogs func(ctx context.Context, deploymentID string, build bool) ([]railway.LogEntry, error),
	out, errOut io.Writer,
) error {
	if out == nil {
		out = os.Stdout
	}
	if errOut == nil {
		errOut = os.Stderr
	}

	if isStructuredOutput(globals) {
		payload := LogsOutput{EnvironmentID: environmentID, Results: make([]LogsResult, 0, len(targets))}
		for _, svc := range targets {
			res := LogsResult{Scope: "service", Service: svc.Name, ServiceID: svc.ID, Build: build, Lines: lines}
			deployments, err := listLatest(ctx, environmentID, svc.ID)
			if err != nil {
				res.Error = fmt.Sprintf("listing deployments: %v", err)
				payload.Results = append(payload.Results, res)
				continue
			}
			if len(deployments) == 0 {
				res.Error = "no deployments found"
				payload.Results = append(payload.Results, res)
				continue
			}
			res.DeploymentID = deployments[0].ID
			entries, err := fetchLogs(ctx, res.DeploymentID, build)
			if err != nil {
				res.Error = err.Error()
				payload.Results = append(payload.Results, res)
				continue
			}
			res.Entries = make([]LogEntryOut, 0, len(entries))
			for _, e := range entries {
				res.Entries = append(res.Entries, LogEntryOut{Timestamp: e.Timestamp, Severity: severity(e.Severity), Message: e.Message})
			}
			payload.Results = append(payload.Results, res)
		}
		return writeStructured(out, globals.Output, payload)
	}

	for _, svc := range targets {
		deployments, err := listLatest(ctx, environmentID, svc.ID)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "failed to list deployments for %s: %v\n", svc.Name, err)
			continue
		}
		if len(deployments) == 0 {
			_, _ = fmt.Fprintf(errOut, "no deployments found for %s\n", svc.Name)
			continue
		}
		entries, err := fetchLogs(ctx, deployments[0].ID, build)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "failed to fetch logs for %s: %v\n", svc.Name, err)
			continue
		}
		for _, e := range entries {
			_, _ = fmt.Fprintf(out, "[%s] %s %s %s\n", svc.Name, e.Timestamp, severity(e.Severity), e.Message)
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
