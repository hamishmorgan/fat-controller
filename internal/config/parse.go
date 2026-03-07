package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// ParseFile reads a TOML file and returns the desired config.
func ParseFile(path string) (*DesiredConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	return Parse(data)
}

// Parse decodes TOML bytes into a DesiredConfig.
func Parse(data []byte) (*DesiredConfig, error) {
	var cfg DesiredConfig
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("parsing TOML: %w", err)
	}

	// Validate: every service must have a name.
	for i, svc := range cfg.Services {
		if svc.Name == "" {
			return nil, fmt.Errorf("service at index %d has no name", i)
		}
	}

	return &cfg, nil
}
