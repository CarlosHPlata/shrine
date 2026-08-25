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

type pathSource struct {
	name  string
	value string
}

func resolvePath(sources []pathSource, missingErr string) (string, error) {
	for _, s := range sources {
		if s.value == "" {
			continue
		}
		resolved, err := expandTilde(s.value)
		if err != nil {
			return "", fmt.Errorf("resolving %s: %w", s.name, err)
		}
		return resolved, nil
	}
	return "", errors.New(missingErr)
}
