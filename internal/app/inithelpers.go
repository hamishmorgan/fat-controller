package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

// EnsureGitignoreHasLine ensures the target line is present in .gitignore.
// Returns true if the line was added.
func EnsureGitignoreHasLine(dir, line string) (bool, error) {
	gitignorePath := filepath.Join(dir, ".gitignore")

	b, err := os.ReadFile(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(gitignorePath, []byte(line+"\n"), 0o644); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, err
	}

	lines := strings.Split(string(b), "\n")
	for _, existing := range lines {
		if strings.TrimSpace(existing) == line {
			return false, nil
		}
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	if len(b) > 0 && b[len(b)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return false, err
		}
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		return false, err
	}
	return true, nil
}

// RenderEnvFile generates a .env file with KEY=VALUE lines for each secret
// detected in the live config. Returns empty string if no secrets found.
func RenderEnvFile(cfg *config.LiveConfig) string {
	secrets := config.CollectSecrets(*cfg)
	if len(secrets) == 0 {
		return ""
	}

	keys := make([]string, 0, len(secrets))
	for k := range secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out strings.Builder
	out.WriteString("# Secret values for fat-controller (gitignored).\n")
	out.WriteString("# Load into your environment before running config apply.\n")
	out.WriteString("# e.g. source fat-controller.secrets\n\n")
	for _, k := range keys {
		_, _ = fmt.Fprintf(&out, "%s=%s\n", k, secrets[k])
	}
	return out.String()
}
