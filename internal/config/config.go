package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the global configuration for shrine.
type Config struct {
	Registries []RegistryConfig `yaml:"registries,omitempty"`
}

// RegistryConfig holds credentials and host information for a Docker registry.
type RegistryConfig struct {
	Host     string `yaml:"host,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// Load reads the config.yml from the specified directory.
// If the file does not exist, it returns an empty Config without error.
func Load(configDir string) (*Config, error) {
	configPath := filepath.Join(configDir, "config.yml")

	data, err := os.ReadFile(configPath)
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
