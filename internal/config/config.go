package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

var validAliasPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Config represents the global configuration for shrine.
type Config struct {
	Registries []RegistryConfig `yaml:"registries,omitempty"`
	SpecsDir   string           `yaml:"specsDir,omitempty"`
	TeamsDir   string           `yaml:"teamsDir,omitempty"`
	Plugins    PluginsConfig    `yaml:"plugins,omitempty"`
}

type PluginsConfig struct {
	Gateway GatewayPluginsConfig `yaml:"gateway,omitempty"`
	Secrets SecretsPluginsConfig `yaml:"secrets,omitempty"`
}

type SecretsPluginsConfig struct {
	Infisical *InfisicalPluginConfig `yaml:"infisical,omitempty"`
}

type GatewayPluginsConfig struct {
	Traefik *TraefikPluginConfig `yaml:"traefik,omitempty"`
}

// RegistryConfig holds credentials and host information for a Docker registry.
type RegistryConfig struct {
	Host     string `yaml:"host,omitempty"`
	Alias    string `yaml:"alias,omitempty"`
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

	if err := cfg.validateSecretsPlugins(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validateSecretsPlugins returns an error if more than one secrets plugin block
// is declared in shrine.yml.
func (c *Config) validateSecretsPlugins() error {
	count := 0
	if c.Plugins.Secrets.Infisical != nil {
		count++
	}
	if count > 1 {
		return fmt.Errorf("plugins.secrets: only one secrets plugin may be active at a time; multiple providers declared")
	}
	return nil
}

// ValidateRegistries checks that all registry aliases are unique and well-formed.
// Entries without an alias are skipped. Must be called explicitly by callers after Load.
func (c *Config) ValidateRegistries() error {
	seen := make(map[string]struct{}, len(c.Registries))
	for _, r := range c.Registries {
		if r.Alias == "" {
			continue
		}
		if !validAliasPattern.MatchString(r.Alias) {
			return fmt.Errorf("registries: alias %q contains invalid characters (alphanumeric, hyphens, underscores only)", r.Alias)
		}
		if _, dup := seen[r.Alias]; dup {
			return fmt.Errorf("registries: alias %q is defined more than once", r.Alias)
		}
		seen[r.Alias] = struct{}{}
	}
	return nil
}

// ResolveSpecsDir returns the specs directory to use, with the following priority:
//  1. flagValue (from --path / -p)
//  2. c.SpecsDir (from config.yml specsDir)
//  3. error if neither is set
//
// Tilde (~) at the start of a path is expanded to the user's home directory.
func (c *Config) ResolveSpecsDir(flagValue string) (string, error) {
	return resolvePath(
		[]string{flagValue, c.SpecsDir},
		"no specs directory: set --path/-p flag or specsDir in config.yml",
	)
}

// ResolveTeamsDir returns the directory to scan for team manifests, with priority:
//  1. flagValue (from --path / -p)
//  2. c.TeamsDir (from config.yml teamsDir)
//  3. c.SpecsDir (from config.yml specsDir)
//  4. error if none is set
func (c *Config) ResolveTeamsDir(flagValue string) (string, error) {
	return resolvePath(
		[]string{flagValue, c.TeamsDir, c.SpecsDir},
		"no specs directory: set --path/-p flag, teamsDir or specsDir in config.yml",
	)
}
