# Project/Environment List & Interactive Picker

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `project list` and `environment list` commands, an interactive picker for project/environment selection when flags are missing, and auto-select when there's only one option.

**Architecture:** Add two new CLI command groups (`project list`, `environment list`) that render
project/environment data in text/json/toml formats. Extract a reusable `prompt` package wrapping
`charmbracelet/huh` Select for interactive picking. Modify `ResolveProjectEnvironment` to auto-select
when unambiguous, or prompt interactively when a TTY is attached, or error with helpful listing when
non-interactive.

**Tech Stack:** Go, charmbracelet/huh (Select), kong, genqlient, httptest

---

## Task 1: Add `charmbracelet/huh` dependency

**Files:**

- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Add the dependency**

Run: `go get github.com/charmbracelet/huh@latest`

Expected: `go.mod` updated with `github.com/charmbracelet/huh` in require block.

**Step 2: Tidy**

Run: `go mod tidy`

Expected: Clean `go.mod` and `go.sum`.

**Step 3: Verify build**

Run: `go build ./...`

Expected: No errors.

**Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "Add charmbracelet/huh dependency for interactive prompts"
```

---

## Task 2: Create prompt package with Select picker

**Files:**

- Create: `internal/prompt/pick.go`
- Test: `internal/prompt/pick_test.go`

**Step 1: Write the failing test**

Create `internal/prompt/pick_test.go`:

```go
package prompt

import "testing"

func TestPickItem_AutoSelectsSingleOption(t *testing.T) {
	items := []Item{{Name: "production", ID: "env-1"}}
	got, err := pickItem("environment", items, false)
	if err != nil {
		t.Fatalf("pickItem() error: %v", err)
	}
	if got != "env-1" {
		t.Errorf("got %q, want %q", got, "env-1")
	}
}

func TestPickItem_ErrorsOnEmptyList(t *testing.T) {
	_, err := pickItem("project", nil, false)
	if err == nil {
		t.Fatal("expected error for empty list")
	}
}

func TestPickItem_NonInteractiveMultipleOptions(t *testing.T) {
	items := []Item{
		{Name: "staging", ID: "env-1"},
		{Name: "production", ID: "env-2"},
	}
	_, err := pickItem("environment", items, false)
	if err == nil {
		t.Fatal("expected error when multiple options and non-interactive")
	}
}
```

**Step 2: Run the test to verify it fails**

Run: `go test ./internal/prompt -run TestPickItem -v`

Expected: FAIL with compilation error (package doesn't exist).

**Step 3: Write minimal implementation**

Create `internal/prompt/pick.go`:

```go
package prompt

import (
	"fmt"

	"github.com/charmbracelet/huh"
)

// Item represents a selectable item with a display name and an ID.
type Item struct {
	Name string
	ID   string
}

// pickItem selects an item from the list:
// - 0 items: error
// - 1 item: auto-select
// - multiple + interactive: huh Select picker
// - multiple + non-interactive: error with listing
func pickItem(label string, items []Item, interactive bool) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("no %ss found", label)
	}
	if len(items) == 1 {
		return items[0].ID, nil
	}
	if !interactive {
		return "", ambiguousError(label, items)
	}
	return runPicker(label, items)
}

func ambiguousError(label string, items []Item) error {
	msg := fmt.Sprintf("multiple %ss available — specify with --%s flag:\n", label, label)
	for _, item := range items {
		msg += fmt.Sprintf("  %s (%s)\n", item.Name, item.ID)
	}
	return fmt.Errorf("%s", msg)
}

func runPicker(label string, items []Item) (string, error) {
	var selected string
	opts := make([]huh.Option[string], len(items))
	for i, item := range items {
		opts[i] = huh.NewOption(item.Name, item.ID)
	}
	err := huh.NewSelect[string]().
		Title(fmt.Sprintf("Select a %s:", label)).
		Options(opts...).
		Value(&selected).
		Run()
	if err != nil {
		return "", err
	}
	return selected, nil
}

// PickProject selects a project from the list.
func PickProject(items []Item, interactive bool) (string, error) {
	return pickItem("project", items, interactive)
}

// PickEnvironment selects an environment from the list.
func PickEnvironment(items []Item, interactive bool) (string, error) {
	return pickItem("environment", items, interactive)
}
```

**Step 4: Run the test to verify it passes**

Run: `go test ./internal/prompt -run TestPickItem -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/prompt/pick.go internal/prompt/pick_test.go
git commit -m "Add prompt package with interactive picker and auto-select"
```

---

## Task 3: Add TTY detection helper

**Files:**

- Create: `internal/prompt/tty.go`
- Test: `internal/prompt/tty_test.go`

**Step 1: Write the failing test**

Create `internal/prompt/tty_test.go`:

```go
package prompt

import (
	"os"
	"testing"
)

func TestIsInteractive_ReturnsBool(t *testing.T) {
	// In test environment, stdin is typically not a TTY.
	got := IsInteractive(os.Stdin)
	// We don't assert true/false since it depends on environment,
	// but verify it doesn't panic and returns a bool.
	_ = got
}
```

**Step 2: Run the test to verify it fails**

Run: `go test ./internal/prompt -run TestIsInteractive -v`

Expected: FAIL with `undefined: IsInteractive`.

**Step 3: Write minimal implementation**

Create `internal/prompt/tty.go`:

```go
package prompt

import (
	"os"

	"github.com/charmbracelet/x/term"
)

// IsInteractive checks if the given file is a terminal.
func IsInteractive(f *os.File) bool {
	return term.IsTerminal(f.Fd())
}
```

Note: `charmbracelet/x/term` is already a dependency (used by the help printer).

**Step 4: Run the test to verify it passes**

Run: `go test ./internal/prompt -run TestIsInteractive -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/prompt/tty.go internal/prompt/tty_test.go
git commit -m "Add TTY detection helper for interactive mode"
```

---

## Task 4: Add `project list` command

**Files:**

- Modify: `internal/cli/cli.go`
- Create: `internal/cli/project_list.go`
- Test: `internal/cli/project_list_test.go`

**Step 1: Write the failing test**

Create `internal/cli/project_list_test.go`:

```go
package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
)

type fakeProjectLister struct {
	projects []cli.ProjectInfo
}

func (f *fakeProjectLister) ListProjects(ctx context.Context) ([]cli.ProjectInfo, error) {
	return f.projects, nil
}

func TestRunProjectList_Text(t *testing.T) {
	lister := &fakeProjectLister{
		projects: []cli.ProjectInfo{
			{ID: "proj-1", Name: "my-app"},
			{ID: "proj-2", Name: "my-api"},
		},
	}
	var buf bytes.Buffer
	err := cli.RunProjectList(context.Background(), &cli.Globals{Output: "text"}, lister, &buf)
	if err != nil {
		t.Fatalf("RunProjectList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "my-app") {
		t.Errorf("expected my-app in output, got:\n%s", got)
	}
	if !strings.Contains(got, "my-api") {
		t.Errorf("expected my-api in output, got:\n%s", got)
	}
}

func TestRunProjectList_JSON(t *testing.T) {
	lister := &fakeProjectLister{
		projects: []cli.ProjectInfo{
			{ID: "proj-1", Name: "my-app"},
		},
	}
	var buf bytes.Buffer
	err := cli.RunProjectList(context.Background(), &cli.Globals{Output: "json"}, lister, &buf)
	if err != nil {
		t.Fatalf("RunProjectList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"id"`) {
		t.Errorf("expected JSON with id field, got:\n%s", got)
	}
}

func TestRunProjectList_Empty(t *testing.T) {
	lister := &fakeProjectLister{}
	var buf bytes.Buffer
	err := cli.RunProjectList(context.Background(), &cli.Globals{Output: "text"}, lister, &buf)
	if err != nil {
		t.Fatalf("RunProjectList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "No projects") {
		t.Errorf("expected 'No projects' message, got:\n%s", got)
	}
}
```

**Step 2: Run the test to verify it fails**

Run: `go test ./internal/cli -run TestRunProjectList -v`

Expected: FAIL with compilation errors.

**Step 3: Write minimal implementation**

Create `internal/cli/project_list.go`:

```go
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// ProjectInfo is a simplified project record for display.
type ProjectInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// projectLister abstracts project listing for tests.
type projectLister interface {
	ListProjects(ctx context.Context) ([]ProjectInfo, error)
}

type defaultProjectLister struct {
	client *railway.Client
}

func (d *defaultProjectLister) ListProjects(ctx context.Context) ([]ProjectInfo, error) {
	resp, err := railway.Projects(ctx, d.client.GQL())
	if err != nil {
		return nil, err
	}
	var projects []ProjectInfo
	for _, edge := range resp.Projects.Edges {
		projects = append(projects, ProjectInfo{
			ID:   edge.Node.Id,
			Name: edge.Node.Name,
		})
	}
	return projects, nil
}

// RunProjectList is the testable core of `project list`.
func RunProjectList(ctx context.Context, globals *Globals, lister projectLister, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}
	projects, err := lister.ListProjects(ctx)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		fmt.Fprintln(out, "No projects found.")
		return nil
	}
	switch globals.Output {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(projects)
	default:
		for _, p := range projects {
			fmt.Fprintf(out, "%s  %s\n", p.Name, p.ID)
		}
		return nil
	}
}

// Run implements `project list`.
func (c *ProjectListCmd) Run(globals *Globals) error {
	store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
	resolved, err := auth.ResolveAuth(globals.Token, store)
	if err != nil {
		return err
	}
	client := railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient())
	lister := &defaultProjectLister{client: client}
	return RunProjectList(context.Background(), globals, lister, os.Stdout)
}
```

Update `internal/cli/cli.go` — add `ProjectCmd` to `CLI`:

```go
type CLI struct {
	Globals `kong:"embed"`

	Auth        AuthCmd        `cmd:"" help:"Manage authentication."`
	Config      ConfigCmd      `cmd:"" name:"config" help:"Declarative configuration management."`
	Project     ProjectCmd     `cmd:"" help:"Manage projects."`
}

type ProjectCmd struct {
	List ProjectListCmd `cmd:"" help:"List available projects."`
}

type ProjectListCmd struct{}
```

**Step 4: Run the test to verify it passes**

Run: `go test ./internal/cli -run TestRunProjectList -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/cli/project_list.go internal/cli/project_list_test.go internal/cli/cli.go
git commit -m "Add project list command"
```

---

## Task 5: Add `environment list` command

**Files:**

- Modify: `internal/cli/cli.go`
- Create: `internal/cli/environment_list.go`
- Test: `internal/cli/environment_list_test.go`

**Step 1: Write the failing test**

Create `internal/cli/environment_list_test.go`:

```go
package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
)

type fakeEnvironmentLister struct {
	environments []cli.EnvironmentInfo
}

func (f *fakeEnvironmentLister) ListEnvironments(ctx context.Context, projectID string) ([]cli.EnvironmentInfo, error) {
	return f.environments, nil
}

func TestRunEnvironmentList_Text(t *testing.T) {
	lister := &fakeEnvironmentLister{
		environments: []cli.EnvironmentInfo{
			{ID: "env-1", Name: "production"},
			{ID: "env-2", Name: "staging"},
		},
	}
	var buf bytes.Buffer
	err := cli.RunEnvironmentList(context.Background(), &cli.Globals{Output: "text"}, "proj-1", lister, &buf)
	if err != nil {
		t.Fatalf("RunEnvironmentList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "production") {
		t.Errorf("expected production in output, got:\n%s", got)
	}
}

func TestRunEnvironmentList_JSON(t *testing.T) {
	lister := &fakeEnvironmentLister{
		environments: []cli.EnvironmentInfo{
			{ID: "env-1", Name: "production"},
		},
	}
	var buf bytes.Buffer
	err := cli.RunEnvironmentList(context.Background(), &cli.Globals{Output: "json"}, "proj-1", lister, &buf)
	if err != nil {
		t.Fatalf("RunEnvironmentList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"id"`) {
		t.Errorf("expected JSON with id field, got:\n%s", got)
	}
}

func TestRunEnvironmentList_Empty(t *testing.T) {
	lister := &fakeEnvironmentLister{}
	var buf bytes.Buffer
	err := cli.RunEnvironmentList(context.Background(), &cli.Globals{Output: "text"}, "proj-1", lister, &buf)
	if err != nil {
		t.Fatalf("RunEnvironmentList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "No environments") {
		t.Errorf("expected 'No environments' message, got:\n%s", got)
	}
}
```

**Step 2: Run the test to verify it fails**

Run: `go test ./internal/cli -run TestRunEnvironmentList -v`

Expected: FAIL with compilation errors.

**Step 3: Write minimal implementation**

Create `internal/cli/environment_list.go`:

```go
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// EnvironmentInfo is a simplified environment record for display.
type EnvironmentInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// environmentLister abstracts environment listing for tests.
type environmentLister interface {
	ListEnvironments(ctx context.Context, projectID string) ([]EnvironmentInfo, error)
}

type defaultEnvironmentLister struct {
	client *railway.Client
}

func (d *defaultEnvironmentLister) ListEnvironments(ctx context.Context, projectID string) ([]EnvironmentInfo, error) {
	resp, err := railway.Environments(ctx, d.client.GQL(), projectID)
	if err != nil {
		return nil, err
	}
	var envs []EnvironmentInfo
	for _, edge := range resp.Environments.Edges {
		envs = append(envs, EnvironmentInfo{
			ID:   edge.Node.Id,
			Name: edge.Node.Name,
		})
	}
	return envs, nil
}

// RunEnvironmentList is the testable core of `environment list`.
func RunEnvironmentList(ctx context.Context, globals *Globals, projectID string, lister environmentLister, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}
	envs, err := lister.ListEnvironments(ctx, projectID)
	if err != nil {
		return err
	}
	if len(envs) == 0 {
		fmt.Fprintln(out, "No environments found.")
		return nil
	}
	switch globals.Output {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(envs)
	default:
		for _, e := range envs {
			fmt.Fprintf(out, "%s  %s\n", e.Name, e.ID)
		}
		return nil
	}
}

// Run implements `environment list`.
// Requires --project flag (or env var) to know which project to list environments for.
func (c *EnvironmentListCmd) Run(globals *Globals) error {
	if globals.Project == "" {
		return fmt.Errorf("--project is required for environment list")
	}
	store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
	resolved, err := auth.ResolveAuth(globals.Token, store)
	if err != nil {
		return err
	}
	client := railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient())

	// Resolve project name to ID if needed.
	projID := globals.Project
	if !isUUID(projID) {
		projLister := &defaultProjectLister{client: client}
		projects, err := projLister.ListProjects(context.Background())
		if err != nil {
			return err
		}
		found := false
		for _, p := range projects {
			if p.Name == globals.Project {
				projID = p.ID
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("project not found: %s", globals.Project)
		}
	}

	lister := &defaultEnvironmentLister{client: client}
	return RunEnvironmentList(context.Background(), globals, projID, lister, os.Stdout)
}
```

Add a small UUID helper to `internal/cli/environment_list.go` (or a shared file):

```go
import "regexp"

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isUUID(s string) bool {
	return uuidRE.MatchString(s)
}
```

Update `internal/cli/cli.go` — add `EnvironmentCmd` to `CLI`:

```go
type CLI struct {
	Globals `kong:"embed"`

	Auth        AuthCmd        `cmd:"" help:"Manage authentication."`
	Config      ConfigCmd      `cmd:"" name:"config" help:"Declarative configuration management."`
	Project     ProjectCmd     `cmd:"" help:"Manage projects."`
	Environment EnvironmentCmd `cmd:"" help:"Manage environments."`
}

type EnvironmentCmd struct {
	List EnvironmentListCmd `cmd:"" help:"List environments for a project."`
}

type EnvironmentListCmd struct{}
```

**Step 4: Run the test to verify it passes**

Run: `go test ./internal/cli -run TestRunEnvironmentList -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/cli/environment_list.go internal/cli/environment_list_test.go internal/cli/cli.go
git commit -m "Add environment list command"
```

---

## Task 6: Update ResolveProjectEnvironment with interactive picker

This is the core behavior change: when project/environment are missing and a TTY is
attached, fetch the list, auto-select if only one, or prompt interactively.

**Files:**

- Modify: `internal/railway/resolve.go`
- Test: `internal/railway/resolve_test.go`

**Step 1: Write the failing tests**

Append to `internal/railway/resolve_test.go`:

```go
func TestResolveProjectEnvironment_AutoSelectsSingleProject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body struct{ Query string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.Contains(body.Query, "projects("):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"projects": map[string]any{
						"edges": []map[string]any{{
							"node": map[string]any{"id": "proj-1", "name": "my-app"},
						}},
					},
				},
			})
		case strings.Contains(body.Query, "environments("):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"environments": map[string]any{
						"edges": []map[string]any{{
							"node": map[string]any{"id": "env-1", "name": "production"},
						}},
					},
				},
			})
		}
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, &auth.ResolvedAuth{
		Source:      auth.SourceEnvAPIToken,
		HeaderName:  "Authorization",
		HeaderValue: "Bearer test",
	}, nil, nil)

	// With no project/environment specified, should auto-select the only option.
	projID, envID, err := railway.ResolveProjectEnvironment(ctx, client, "", "")
	if err != nil {
		t.Fatalf("ResolveProjectEnvironment() error: %v", err)
	}
	if projID != "proj-1" {
		t.Errorf("projID = %q, want proj-1", projID)
	}
	if envID != "env-1" {
		t.Errorf("envID = %q, want env-1", envID)
	}
}

func TestResolveProjectEnvironment_ErrorsOnAmbiguousNonInteractive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"projects": map[string]any{
					"edges": []map[string]any{
						{"node": map[string]any{"id": "proj-1", "name": "app-1"}},
						{"node": map[string]any{"id": "proj-2", "name": "app-2"}},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, &auth.ResolvedAuth{
		Source:      auth.SourceEnvAPIToken,
		HeaderName:  "Authorization",
		HeaderValue: "Bearer test",
	}, nil, nil)

	// Multiple projects, no flag, non-interactive: should error with listing.
	_, _, err := railway.ResolveProjectEnvironment(ctx, client, "", "")
	if err == nil {
		t.Fatal("expected error for ambiguous project")
	}
	if !strings.Contains(err.Error(), "app-1") || !strings.Contains(err.Error(), "app-2") {
		t.Errorf("expected error to list projects, got: %v", err)
	}
}
```

**Step 2: Run the tests to verify they fail**

Run: `go test ./internal/railway -run TestResolveProjectEnvironment_AutoSelects -v`

Expected: FAIL (current code returns "project and environment required").

**Step 3: Modify implementation**

Update `internal/railway/resolve.go`:

The key change is to `ResolveProjectEnvironment` — when project/environment are empty
(account token), instead of immediately erroring, fetch the list, auto-select if single,
prompt if interactive, or error with helpful listing.

```go
package railway

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ResolveProjectEnvironment(ctx context.Context, client *Client, project, environment string) (string, string, error) {
	if client == nil || client.Auth() == nil {
		return "", "", errors.New("missing auth")
	}
	if client.Auth().Source == auth.SourceEnvToken {
		resp, err := ProjectToken(ctx, client.GQL())
		if err != nil {
			return "", "", err
		}
		return resp.ProjectToken.ProjectId, resp.ProjectToken.EnvironmentId, nil
	}

	projID, err := resolveProjectID(ctx, client, project)
	if err != nil {
		return "", "", err
	}
	envID, err := resolveEnvironmentID(ctx, client, projID, environment)
	if err != nil {
		return "", "", err
	}
	return projID, envID, nil
}

func resolveProjectID(ctx context.Context, client *Client, project string) (string, error) {
	if project != "" && uuidPattern.MatchString(project) {
		return project, nil
	}
	resp, err := Projects(ctx, client.GQL())
	if err != nil {
		return "", err
	}

	// If a name was given, find it.
	if project != "" {
		for _, edge := range resp.Projects.Edges {
			if edge.Node.Name == project {
				return edge.Node.Id, nil
			}
		}
		return "", fmt.Errorf("project not found: %s", project)
	}

	// No project specified — try auto-select or prompt.
	items := make([]prompt.Item, len(resp.Projects.Edges))
	for i, edge := range resp.Projects.Edges {
		items[i] = prompt.Item{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	return prompt.PickProject(items, prompt.StdinIsInteractive())
}

func resolveEnvironmentID(ctx context.Context, client *Client, projectID, env string) (string, error) {
	if env != "" && uuidPattern.MatchString(env) {
		return env, nil
	}
	resp, err := Environments(ctx, client.GQL(), projectID)
	if err != nil {
		return "", err
	}

	// If a name was given, find it.
	if env != "" {
		for _, edge := range resp.Environments.Edges {
			if edge.Node.Name == env {
				return edge.Node.Id, nil
			}
		}
		return "", fmt.Errorf("environment not found: %s", env)
	}

	// No environment specified — try auto-select or prompt.
	items := make([]prompt.Item, len(resp.Environments.Edges))
	for i, edge := range resp.Environments.Edges {
		items[i] = prompt.Item{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	return prompt.PickEnvironment(items, prompt.StdinIsInteractive())
}
```

Add to `internal/prompt/pick.go`:

```go
// StdinIsInteractive checks if os.Stdin is a TTY.
func StdinIsInteractive() bool {
	return IsInteractive(os.Stdin)
}
```

**Step 4: Run the tests to verify they pass**

Run: `go test ./internal/railway -run TestResolveProjectEnvironment -v`

Expected: PASS for all resolve tests.

**Step 5: Commit**

```bash
git add internal/railway/resolve.go internal/prompt/pick.go
git commit -m "Auto-select or prompt for project/environment when not specified"
```

---

## Task 7: Update docs

**Files:**

- Modify: `docs/COMMANDS.md`

**Step 1: Update docs**

Add the new commands to `docs/COMMANDS.md`:

```markdown
# Project / environment discovery
fat-controller project list                       # list available projects
fat-controller environment list --project my-app  # list environments for a project
```

Add a note about auto-selection behavior:

```markdown
## Project and environment resolution

When using an account-level token (`RAILWAY_API_TOKEN` or stored OAuth),
`config get/set/delete` need a project and environment. Resolution works as:

1. If `--project`/`--environment` flags (or env vars) are set, use them.
2. If only one project/environment exists, auto-select it.
3. If a TTY is attached, show an interactive picker.
4. Otherwise, error with a listing of available options.

Use `project list` and `environment list` to discover available options.
```

**Step 2: Run lint**

Run: `mise run lint:md`

Expected: PASS.

**Step 3: Commit**

```bash
git add docs/COMMANDS.md
git commit -m "Document project/environment list commands and resolution behavior"
```

---

## Task 8: Final verification

**Files:**

- Test: `./...`

**Step 1: Run the full check suite**

Run: `mise run check`

Expected: All linters pass, all tests pass, build succeeds.

**Step 2: Run targeted tests**

Run: `go test ./internal/prompt ./internal/cli ./internal/railway -v`

Expected: PASS for all.

**Step 3: Commit if any remaining changes**

```bash
git add -A
git commit -m "Complete project/environment list and interactive picker"
```
