package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// Paths holds the resolved configuration and state directories.
type Paths struct {
	ConfigDir string
	StateDir  string
}

// ResolvePaths determines the configuration and state directories based on precedence:
// Flags > Environment Variables > .env File > Standard Paths (XDG or System).
func ResolvePaths(flagConfig, flagState string) (*Paths, error) {
	// Load .env file from current directory if it exists.
	// We ignore the error because it's okay if the file is missing.
	_ = godotenv.Load()

	p := &Paths{}

	resolveConfigDir(flagConfig, p)
	resolveStateDir(flagState, p)

	// Ensure StateDir exists (ConfigDir might be empty/non-existent if no config is used yet)
	if err := os.MkdirAll(p.StateDir, 0755); err != nil {
		return nil, fmt.Errorf("ensuring state directory exists at %q: %w", p.StateDir, err)
	}

	return p, nil
}

func resolveStateDir(flagState string, p *Paths) {
	if flagState != "" {
		p.StateDir = flagState
	} else if env := os.Getenv("SHRINE_STATE_DIR"); env != "" {
		p.StateDir = env
	} else {
		p.StateDir = defaultStateDir()
	}
}

func resolveConfigDir(flagConfig string, p *Paths) {
	if flagConfig != "" {
		p.ConfigDir = flagConfig
	} else if env := os.Getenv("SHRINE_CONFIG_DIR"); env != "" {
		p.ConfigDir = env
	} else {
		p.ConfigDir = defaultConfigDir()
	}
}

func defaultConfigDir() string {
	// If root, use /etc/shrine
	if os.Getuid() == 0 {
		return "/etc/shrine"
	}

	// Fallback to XDG_CONFIG_HOME or ~/.config/shrine
	configDir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".config", "shrine")
	}
	return filepath.Join(configDir, "shrine")
}

func defaultStateDir() string {
	// If root, use /var/lib/shrine
	if os.Getuid() == 0 {
		return "/var/lib/shrine"
	}

	// Fallback to XDG_DATA_HOME or ~/.local/share/shrine
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/shrine-state" // Last resort
	}

	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome != "" {
		return filepath.Join(dataHome, "shrine")
	}

	return filepath.Join(home, ".local", "share", "shrine")
}
