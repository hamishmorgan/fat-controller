package config

import (
	"errors"
	"strings"
)

// Path is a parsed dot-path like "service.section.key".
type Path struct {
	Service string
	Section string
	Key     string
}

// ParsePath parses a dot-path into components.
func ParsePath(input string) (Path, error) {
	if strings.TrimSpace(input) == "" {
		return Path{}, errors.New("path cannot be empty")
	}
	parts := strings.Split(input, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return Path{}, errors.New("path must have 1 to 3 segments")
	}
	for _, p := range parts {
		if p == "" {
			return Path{}, errors.New("path segments cannot be empty")
		}
	}
	path := Path{Service: parts[0]}
	if len(parts) > 1 {
		path.Section = parts[1]
	}
	if len(parts) > 2 {
		path.Key = parts[2]
	}
	return path, nil
}
