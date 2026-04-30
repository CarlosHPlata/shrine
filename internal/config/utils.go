package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

func resolvePath(candidates []string, missingErr string) (string, error) {
	for _, c := range candidates {
		if c != "" {
			return expandTilde(c)
		}
	}
	return "", errors.New(missingErr)
}
