package config

import (
	"os"
	"path/filepath"
	"strings"
)

var configCandidates = []string{
	"fat-controller.toml",
	filepath.Join(".config", "fat-controller.toml"),
	filepath.Join(".config", "fat-controller", "config.toml"),
}

func FindConfigInDir(dir string) string {
	for _, candidate := range configCandidates {
		path := filepath.Join(dir, candidate)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func LocalOverridePath(configPath string) string {
	ext := filepath.Ext(configPath)
	base := strings.TrimSuffix(configPath, ext)
	return base + ".local" + ext
}

func DiscoverConfigs(startDir string) ([]string, error) {
	startDir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}

	gitRoot, err := FindGitRoot(startDir)
	var boundary string
	if err != nil {
		boundary = startDir
	} else {
		boundary = gitRoot
	}

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

	var paths []string
	for _, d := range dirs {
		if p := FindConfigInDir(d); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}
