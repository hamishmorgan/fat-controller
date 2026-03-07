package config

import (
	"errors"
	"os"
	"path/filepath"
)

var ErrNotInGitRepo = errors.New("not in a git repository")

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
