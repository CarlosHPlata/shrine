package handler

import (
	"fmt"
	"os"
	"path/filepath"
)

type ResourceOptions struct {
	Name             string
	Team             string
	OutputDir        string
	Type             string
	Version          string
	ExposeToPlatform bool
}

const resourceSkeleton = `apiVersion: shrine/v1
kind: Resource
metadata:
  name: %s
  owner: %s
spec:
  type: %s
  version: "%s"
  networking:
    exposeToPlatform: %v
`

// GenerateResource creates a skeleton resource manifest YAML file in the given directory.
func GenerateResource(opts ResourceOptions) error {
	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return fmt.Errorf("creating directory %q: %w", opts.OutputDir, err)
	}

	path := filepath.Join(opts.OutputDir, opts.Name+".yml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("resource manifest already exists at %q", path)
	}

	content := fmt.Sprintf(resourceSkeleton,
		opts.Name,
		opts.Team,
		opts.Type,
		opts.Version,
		opts.ExposeToPlatform,
	)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing resource manifest: %w", err)
	}

	fmt.Printf("Created resource manifest: %s\n", path)
	return nil
}
