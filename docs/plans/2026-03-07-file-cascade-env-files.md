# File Cascade and Env Files

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the config file cascade (upward directory walk, global config, local overrides) and env file loading for `${VAR}` interpolation, as described in `docs/ARCHITECTURE.md`.

**Architecture:** Config files are discovered by walking from the working directory upward to the git root. At each level, three paths are checked. All found files are merged shallowest-first. A local override (`.local.toml`) merges on top. The global XDG config merges before everything. Env files supply values for `${VAR}` interpolation — checked before the process environment. `--config-file` disables the walk entirely.

**Tech Stack:** Go 1.26, BurntSushi/toml v1.6.0, `os/exec` for git root detection, standard `testing` library.

**Depends on:** Plan 1 (Config Schema Migration) must be complete.

---

## Context for the implementer

### Current state

`internal/config/load.go` has `LoadConfigs(dir string, extraFiles []string)`:

1. Reads `fat-controller.toml` from `dir` (required — errors if missing)
2. Checks for `fat-controller.local.toml` (deprecated warning only)
3. Parses extra files from `--config` flags
4. Merges all with `Merge()`

There is no upward walk, no XDG global config, no env file loading.
`${VAR}` interpolation in `interpolate.go` resolves only from `os.LookupEnv`.

### Target state (from ARCHITECTURE.md)

**File cascade (lowest to highest priority):**

1. Compiled-in defaults
2. Global config: `$XDG_CONFIG_HOME/fat-controller/config.toml`
3. Discovered configs: walk upward from cwd to git root, shallowest-first
4. Local override: co-located with primary (deepest) config, `.local` inserted
5. Environment variables
6. CLI flags

**Config file discovery per directory:**

1. `[path]/fat-controller.toml`
2. `[path]/.config/fat-controller.toml`
3. `[path]/.config/fat-controller/config.toml`

First match at each level wins. At most one config file per directory.

**`--config-file` behavior:** Loads only the specified file. No walk. No local override.

**Local override naming:**

- `fat-controller.toml` → `fat-controller.local.toml`
- `.config/fat-controller.toml` → `.config/fat-controller.local.toml`
- `.config/fat-controller/config.toml` → `.config/fat-controller/config.local.toml`

**Env file loading:**

- Declared via `tool.env_file`, `--env-file`, or `FAT_CONTROLLER_ENV_FILE`
- Dotenv format (`KEY=value`, one per line)
- Paths relative to the config file that declares them
- `${VAR}` resolution order: env files (first match wins) → process env → error

### Key files

| File | Role |
|------|------|
| `internal/config/load.go` | File discovery + loading pipeline |
| `internal/config/interpolate.go` | `${VAR}` resolution |
| `internal/platform/paths.go` | XDG path helpers |

### Testing conventions

Same as Plan 1. Use `t.TempDir()` for filesystem tests. Use `t.Setenv()`
for env var tests. External test package (`package config_test`).

---

## Task 1: Implement git root detection

**Files:**

- Create: `internal/config/gitroot.go`
- Test: `internal/config/gitroot_test.go`

### Step 1: Write the failing test

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestFindGitRoot_InRepo(t *testing.T) {
	// Create a temp dir with a .git directory.
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := config.FindGitRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Errorf("FindGitRoot(%q) = %q, want %q", sub, got, root)
	}
}

func TestFindGitRoot_NotInRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := config.FindGitRoot(dir)
	if err == nil {
		t.Fatal("expected error when not in a git repo")
	}
}

func TestFindGitRoot_AtRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := config.FindGitRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Errorf("FindGitRoot(%q) = %q, want %q", root, got, root)
	}
}
```

### Step 2: Run test — expect fail

### Step 3: Implement git root detection

```go
package config

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNotInGitRepo is returned when no .git directory is found.
var ErrNotInGitRepo = errors.New("not in a git repository")

// FindGitRoot walks upward from dir looking for a .git directory.
// Returns the directory containing .git, or ErrNotInGitRepo.
func FindGitRoot(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	for {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotInGitRepo
		}
		dir = parent
	}
}
```

### Step 4: Run test — expect pass

### Step 5: Commit

```bash
git add internal/config/gitroot.go internal/config/gitroot_test.go
git commit -m "feat: add FindGitRoot for upward .git directory walk"
```

---

## Task 2: Implement config file discovery (per-directory)

**Files:**

- Create: `internal/config/discover.go`
- Test: `internal/config/discover_test.go`

### Step 1: Write tests for per-directory discovery

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindConfigInDir_FatControllerToml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "fat-controller.toml"), "name = \"test\"")

	got := config.FindConfigInDir(dir)
	want := filepath.Join(dir, "fat-controller.toml")
	if got != want {
		t.Errorf("FindConfigInDir(%q) = %q, want %q", dir, got, want)
	}
}

func TestFindConfigInDir_DotConfigFatControllerToml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".config", "fat-controller.toml"), "name = \"test\"")

	got := config.FindConfigInDir(dir)
	want := filepath.Join(dir, ".config", "fat-controller.toml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFindConfigInDir_DotConfigFatControllerDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".config", "fat-controller", "config.toml"), "name = \"test\"")

	got := config.FindConfigInDir(dir)
	want := filepath.Join(dir, ".config", "fat-controller", "config.toml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFindConfigInDir_PrecedenceOrder(t *testing.T) {
	// If multiple exist, first match wins.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "fat-controller.toml"), "name = \"visible\"")
	writeFile(t, filepath.Join(dir, ".config", "fat-controller.toml"), "name = \"hidden\"")

	got := config.FindConfigInDir(dir)
	want := filepath.Join(dir, "fat-controller.toml")
	if got != want {
		t.Errorf("got %q, want %q (visible should beat hidden)", got, want)
	}
}

func TestFindConfigInDir_NoConfig(t *testing.T) {
	dir := t.TempDir()
	got := config.FindConfigInDir(dir)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestLocalOverridePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"fat-controller.toml", "fat-controller.local.toml"},
		{".config/fat-controller.toml", ".config/fat-controller.local.toml"},
		{".config/fat-controller/config.toml", ".config/fat-controller/config.local.toml"},
	}
	for _, tt := range tests {
		got := config.LocalOverridePath(tt.input)
		if got != tt.want {
			t.Errorf("LocalOverridePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
```

### Step 2: Run tests — expect fail

### Step 3: Implement discovery

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
)

// configCandidates are the paths checked at each directory level,
// in priority order (first match wins).
var configCandidates = []string{
	"fat-controller.toml",
	filepath.Join(".config", "fat-controller.toml"),
	filepath.Join(".config", "fat-controller", "config.toml"),
}

// FindConfigInDir returns the path to the first config file found
// in dir, checking the three candidate paths in order. Returns ""
// if no config file exists.
func FindConfigInDir(dir string) string {
	for _, candidate := range configCandidates {
		path := filepath.Join(dir, candidate)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// LocalOverridePath returns the local override path for a config file.
// Inserts ".local" before the ".toml" extension.
func LocalOverridePath(configPath string) string {
	ext := filepath.Ext(configPath)
	base := strings.TrimSuffix(configPath, ext)
	return base + ".local" + ext
}
```

### Step 4: Run tests — expect pass

### Step 5: Commit

---

## Task 3: Implement the upward walk

Collect all config files from cwd to git root (or cwd only if not
in a git repo).

**Files:**

- Add to: `internal/config/discover.go`
- Add to: `internal/config/discover_test.go`

### Step 1: Write tests for the walk

```go
func TestDiscoverConfigs_WalkToGitRoot(t *testing.T) {
	// Layout:
	//   root/.git/
	//   root/fat-controller.toml         (shallowest)
	//   root/envs/production/fat-controller.toml  (deepest)
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".git"), 0o755)

	deep := filepath.Join(root, "envs", "production")
	os.MkdirAll(deep, 0o755)

	writeFile(t, filepath.Join(root, "fat-controller.toml"), `
[workspace]
name = "Acme"
`)
	writeFile(t, filepath.Join(deep, "fat-controller.toml"), `
name = "production"
`)

	paths, err := config.DiscoverConfigs(deep)
	if err != nil {
		t.Fatal(err)
	}
	// Should be shallowest-first.
	if len(paths) != 2 {
		t.Fatalf("len = %d, want 2", len(paths))
	}
	if paths[0] != filepath.Join(root, "fat-controller.toml") {
		t.Errorf("paths[0] = %q, want root config", paths[0])
	}
	if paths[1] != filepath.Join(deep, "fat-controller.toml") {
		t.Errorf("paths[1] = %q, want deep config", paths[1])
	}
}

func TestDiscoverConfigs_NotInGitRepo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "fat-controller.toml"), "name = \"test\"")

	paths, err := config.DiscoverConfigs(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Only cwd checked when not in git repo.
	if len(paths) != 1 {
		t.Fatalf("len = %d, want 1", len(paths))
	}
}

func TestDiscoverConfigs_NoConfigs(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0o755)

	paths, err := config.DiscoverConfigs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Errorf("len = %d, want 0", len(paths))
	}
}

func TestDiscoverConfigs_SkipsEmptyLevels(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".git"), 0o755)

	// root has config, middle level does not, deep has config.
	deep := filepath.Join(root, "a", "b")
	os.MkdirAll(deep, 0o755)

	writeFile(t, filepath.Join(root, "fat-controller.toml"), "name = \"root\"")
	writeFile(t, filepath.Join(deep, "fat-controller.toml"), "name = \"deep\"")

	paths, err := config.DiscoverConfigs(deep)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("len = %d, want 2", len(paths))
	}
}
```

### Step 2: Run tests — expect fail

### Step 3: Implement DiscoverConfigs

```go
// DiscoverConfigs walks from startDir upward to the git root,
// collecting config files at each level. Returns paths ordered
// shallowest-first (lowest priority first).
//
// If not in a git repo, only startDir is checked.
func DiscoverConfigs(startDir string) ([]string, error) {
	startDir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}

	// Determine the walk boundary.
	gitRoot, err := FindGitRoot(startDir)
	var boundary string
	if err != nil {
		// Not in a git repo — only check startDir.
		boundary = startDir
	} else {
		boundary = gitRoot
	}

	// Collect directories from startDir up to boundary (inclusive).
	var dirs []string
	dir := startDir
	for {
		dirs = append(dirs, dir)
		if dir == boundary {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Reverse so shallowest is first.
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}

	// Find config files at each level.
	var paths []string
	for _, d := range dirs {
		if p := FindConfigInDir(d); p != "" {
			paths = append(paths, p)
		}
	}

	return paths, nil
}
```

### Step 4: Run tests — expect pass

### Step 5: Commit

---

## Task 4: Implement the full cascade loader

Replace `LoadConfigs` with a new `LoadCascade` function that
implements the full cascade.

**Files:**

- Modify: `internal/config/load.go`
- Rewrite: `internal/config/load_test.go`

### Step 1: Write cascade tests

Test cases:

- Basic cascade: global + discovered + local override
- `--config-file` skips walk and local override
- Local override merges on top
- Global config from XDG path
- Primary config is the deepest discovered file

### Step 2: Run tests — expect fail

### Step 3: Implement LoadCascade

```go
// LoadOptions controls config loading behavior.
type LoadOptions struct {
	// ConfigFile overrides discovery. When set, only this file is
	// loaded — no walk, no local override.
	ConfigFile string

	// WorkDir is the starting directory for discovery. Defaults to
	// the process working directory.
	WorkDir string
}

// LoadResult contains the loaded config and metadata about where
// it came from.
type LoadResult struct {
	Config      *DesiredConfig
	PrimaryFile string   // deepest discovered file (where adopt writes)
	Files       []string // all files that were loaded, in merge order
}

// LoadCascade loads and merges config files according to the cascade:
//
//  1. Compiled-in defaults
//  2. Global config ($XDG_CONFIG_HOME/fat-controller/config.toml)
//  3. Discovered configs (walk upward, shallowest-first)
//  4. Local override (co-located with primary, .local.toml)
//  5. (env vars and CLI flags are handled by the caller)
//
// When opts.ConfigFile is set, only that file is loaded.
func LoadCascade(opts LoadOptions) (*LoadResult, error) {
	// ... implementation
}
```

### Step 4: Run tests — expect pass

### Step 5: Commit

---

## Task 5: Implement env file parsing

**Files:**

- Create: `internal/config/envfile.go`
- Test: `internal/config/envfile_test.go`

### Step 1: Write env file parsing tests

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestParseEnvFile_BasicKeyValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	os.WriteFile(path, []byte("KEY=value\nSECRET=s3cret\n"), 0o644)

	vars, err := config.ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if vars["KEY"] != "value" {
		t.Errorf("KEY = %q, want %q", vars["KEY"], "value")
	}
	if vars["SECRET"] != "s3cret" {
		t.Errorf("SECRET = %q, want %q", vars["SECRET"], "s3cret")
	}
}

func TestParseEnvFile_CommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	os.WriteFile(path, []byte("# comment\n\nKEY=value\n  # indented comment\n"), 0o644)

	vars, err := config.ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(vars) != 1 {
		t.Errorf("len = %d, want 1", len(vars))
	}
}

func TestParseEnvFile_QuotedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	os.WriteFile(path, []byte(`
SINGLE='single quoted'
DOUBLE="double quoted"
BARE=bare value
EQUALS=a=b=c
`), 0o644)

	vars, err := config.ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if vars["SINGLE"] != "single quoted" {
		t.Errorf("SINGLE = %q", vars["SINGLE"])
	}
	if vars["DOUBLE"] != "double quoted" {
		t.Errorf("DOUBLE = %q", vars["DOUBLE"])
	}
	if vars["BARE"] != "bare value" {
		t.Errorf("BARE = %q", vars["BARE"])
	}
	if vars["EQUALS"] != "a=b=c" {
		t.Errorf("EQUALS = %q", vars["EQUALS"])
	}
}

func TestParseEnvFile_EmptyValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	os.WriteFile(path, []byte("EMPTY=\n"), 0o644)

	vars, err := config.ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := vars["EMPTY"]; !ok || v != "" {
		t.Errorf("EMPTY = %q, ok = %v", v, ok)
	}
}

func TestParseEnvFile_NonexistentFile(t *testing.T) {
	_, err := config.ParseEnvFile("/nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseEnvFile_ExportPrefix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	os.WriteFile(path, []byte("export KEY=value\n"), 0o644)

	vars, err := config.ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if vars["KEY"] != "value" {
		t.Errorf("KEY = %q, want %q", vars["KEY"], "value")
	}
}
```

### Step 2: Run tests — expect fail

### Step 3: Implement env file parser

```go
package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ParseEnvFile reads a dotenv-format file and returns key-value pairs.
// Supports: KEY=value, KEY="quoted", KEY='quoted', # comments,
// blank lines, export prefix, values containing = signs.
func ParseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening env file: %w", err)
	}
	defer f.Close()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip blanks and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip optional "export " prefix.
		line = strings.TrimPrefix(line, "export ")

		// Split on first =.
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue // malformed line, skip
		}

		key := strings.TrimSpace(line[:idx])
		value := line[idx+1:]

		// Unquote if wrapped in matching quotes.
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		vars[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading env file: %w", err)
	}

	return vars, nil
}
```

### Step 4: Run tests — expect pass

### Step 5: Commit

---

## Task 6: Implement env file loading into interpolation

Update `Interpolate` to accept env file values and check them before
falling back to the process environment.

**Files:**

- Modify: `internal/config/interpolate.go`
- Modify: `internal/config/interpolate_test.go`

### Step 1: Write tests for env-file-aware interpolation

```go
func TestInterpolate_EnvFileBeforeProcessEnv(t *testing.T) {
	t.Setenv("MY_VAR", "from-process")
	envVars := map[string]string{"MY_VAR": "from-file"}

	cfg := &config.DesiredConfig{
		Variables: config.Variables{"KEY": "${MY_VAR}"},
	}
	err := config.Interpolate(cfg, envVars)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Variables["KEY"] != "from-file" {
		t.Errorf("KEY = %q, want %q (env file should win)", cfg.Variables["KEY"], "from-file")
	}
}

func TestInterpolate_FallsBackToProcessEnv(t *testing.T) {
	t.Setenv("MY_VAR", "from-process")

	cfg := &config.DesiredConfig{
		Variables: config.Variables{"KEY": "${MY_VAR}"},
	}
	err := config.Interpolate(cfg, nil) // no env file vars
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Variables["KEY"] != "from-process" {
		t.Errorf("KEY = %q, want %q", cfg.Variables["KEY"], "from-process")
	}
}

func TestInterpolate_MissingVarErrors(t *testing.T) {
	t.Setenv("MY_VAR", "")
	os.Unsetenv("MY_VAR")

	cfg := &config.DesiredConfig{
		Variables: config.Variables{"KEY": "${MISSING_VAR}"},
	}
	err := config.Interpolate(cfg, nil)
	if err == nil {
		t.Fatal("expected error for missing var")
	}
}

func TestInterpolate_RegistryCredentials(t *testing.T) {
	envVars := map[string]string{"REG_PASS": "secret123"}
	cfg := &config.DesiredConfig{
		Services: []*config.DesiredService{{
			Name: "api",
			Deploy: &config.DesiredDeploy{
				RegistryCredentials: &config.RegistryCredentials{
					Username: "deploy",
					Password: "${REG_PASS}",
				},
			},
		}},
	}
	err := config.Interpolate(cfg, envVars)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Services[0].Deploy.RegistryCredentials.Password != "secret123" {
		t.Errorf("Password = %q, want %q", cfg.Services[0].Deploy.RegistryCredentials.Password, "secret123")
	}
}
```

### Step 2: Run tests — expect fail

### Step 3: Update Interpolate signature

Change `Interpolate(cfg *DesiredConfig) error` to
`Interpolate(cfg *DesiredConfig, envFileVars map[string]string) error`.

Resolution order for each `${VAR}`:

1. Check `envFileVars[varName]`
2. Check `os.LookupEnv(varName)`
3. Collect as missing → return error

Also interpolate `RegistryCredentials.Password` if present.

### Step 4: Run tests — expect pass

### Step 5: Commit

---

## Task 7: Wire env file loading into the cascade loader

Load env files declared in `tool.env_file` and pass their values
to `Interpolate`.

**Files:**

- Modify: `internal/config/load.go`
- Add to: `internal/config/load_test.go`

### Step 1: Write integration tests

Test that a cascade with `[tool] env_file = ".env"` in one of the
config files causes that env file to be loaded and used for
interpolation.

### Step 2–5: Standard TDD cycle + commit

---

## Task 8: CI detection

**Files:**

- Modify: `internal/prompt/tty.go`
- Test: `internal/prompt/tty_test.go` (create or extend)

### Step 1: Write test for CI detection

```go
package prompt_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/prompt"
)

func TestIsCI_True(t *testing.T) {
	t.Setenv("CI", "true")
	if !prompt.IsCI() {
		t.Error("IsCI() = false, want true")
	}
}

func TestIsCI_False(t *testing.T) {
	t.Setenv("CI", "")
	if prompt.IsCI() {
		t.Error("IsCI() = true, want false")
	}
}

func TestIsCI_Unset(t *testing.T) {
	// Don't set CI at all.
	if prompt.IsCI() {
		t.Error("IsCI() = true, want false when unset")
	}
}
```

### Step 2: Run tests — expect fail

### Step 3: Implement CI detection

Add to `internal/prompt/tty.go`:

```go
// IsCI returns true when the CI environment variable is set to "true".
// Many CI providers (GitHub Actions, GitLab CI, CircleCI) set this.
func IsCI() bool {
	return os.Getenv("CI") == "true"
}
```

Update `StdinIsInteractive()` to account for CI:

```go
func StdinIsInteractive() bool {
	if IsCI() {
		return false
	}
	return IsInteractive(os.Stdin)
}
```

### Step 4: Run tests — expect pass

### Step 5: Commit

---

## Task 9: COLUMNS support

**Files:**

- Modify: `internal/cli/help.go` (the `guessWidth` function)
- Test: `internal/cli/help_test.go` (extend)

### Step 1: Write test for COLUMNS override

```go
func TestGuessWidth_ColumnsEnvVar(t *testing.T) {
	t.Setenv("COLUMNS", "120")
	got := cli.GuessWidth()
	if got != 120 {
		t.Errorf("GuessWidth() = %d, want 120", got)
	}
}

func TestGuessWidth_InvalidColumns(t *testing.T) {
	t.Setenv("COLUMNS", "not-a-number")
	got := cli.GuessWidth()
	// Should fall back to ioctl or 80.
	if got < 1 {
		t.Errorf("GuessWidth() = %d, want positive", got)
	}
}
```

### Step 2: Run tests — expect fail

### Step 3: Update guessWidth

Add `COLUMNS` check before the ioctl syscall:

```go
func guessWidth() int {
	if s := os.Getenv("COLUMNS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	// ... existing ioctl fallback
}
```

Export as `GuessWidth` for testing, or test via the help output.

### Step 4: Run tests — expect pass

### Step 5: Commit

---

## Task 10: Update all callers of LoadConfigs and Interpolate

Every CLI command that calls `LoadConfigs` or `Interpolate` needs
updating to use the new signatures.

**Files:**

- Modify: `internal/cli/config_common.go`
- Modify: `internal/cli/config_validate.go`
- Modify: `internal/cli/config_init.go`
- Modify affected test files

### Step 1: Update loadAndFetch to use LoadCascade

### Step 2: Update Interpolate callers to pass env file vars

### Step 3: Run full test suite

Run: `go test ./...`

### Step 4: Commit

---

## Task 11: Final verification

### Step 1: Run `mise run check`

### Step 2: Run `go test -race ./...`

### Step 3: Verify binary builds

Run: `go build -o /dev/null ./cmd/fat-controller`
