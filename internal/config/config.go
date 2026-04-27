package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the global configuration for shrine.
type Config struct {
	Registries []RegistryConfig `yaml:"registries,omitempty"`
	SpecsDir   string           `yaml:"specsDir,omitempty"`
}

// RegistryConfig holds credentials and host information for a Docker registry.
type RegistryConfig struct {
	Host     string `yaml:"host,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// Load reads the config file at the given path.
// If the file does not exist, it returns an empty Config without error.
func Load(configFile string) (*Config, error) {
	data, err := os.ReadFile(configFile)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, nil
	}

	if err != nil {
		return nil, err
	}

	// If the file is empty, return an empty struct immediately to avoid unmarshaling empty data.
	if len(data) == 0 {
		return &Config{}, nil
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// ResolveSpecsDir returns the specs directory to use, with the following priority:
//  1. flagValue (from --path / -p)
//  2. c.SpecsDir (from config.yml specsDir)
//  3. error if neither is set
//
// Tilde (~) at the start of a path is expanded to the user's home directory.
func (c *Config) ResolveSpecsDir(flagValue string) (string, error) {
	raw := flagValue
	if raw == "" {
		raw = c.SpecsDir
	}
	if raw == "" {
		return "", fmt.Errorf("no specs directory: set --path/-p flag or specsDir in config.yml")
	}
	return expandTilde(raw)
}

func expandTilde(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expanding ~: %w", err)
	}
	return filepath.Join(home, path[1:]), nil
}
