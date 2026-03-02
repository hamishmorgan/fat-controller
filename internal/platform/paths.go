package platform

import (
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

const appName = "fat-controller"

// ConfigDir returns the path to the app's config directory.
// Does NOT create the directory.
func ConfigDir() string {
	return filepath.Join(xdg.ConfigHome, appName)
}

// AuthFilePath returns the path to the auth token fallback file.
// Does NOT create the file or its parent directory.
func AuthFilePath() string {
	return filepath.Join(xdg.ConfigHome, appName, "auth.json")
}

// ConfigFilePath returns the path to the user config file.
// Does NOT create the file or its parent directory.
func ConfigFilePath() string {
	return filepath.Join(xdg.ConfigHome, appName, "config.toml")
}

// EnsureConfigDir creates the config directory if it doesn't exist
// and returns its path.
func EnsureConfigDir() (string, error) {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
