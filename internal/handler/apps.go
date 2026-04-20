package handler

import (
	"fmt"
	"os"
	"path/filepath"
)

type AppOptions struct {
	Name             string
	Team             string
	OutputDir        string
	Port             int
	Replicas         int
	Domain           string
	PathPrefix       string
	ExposeToPlatform bool
	Image            string
}

const appSkeleton = `apiVersion: shrine/v1
kind: Application
metadata:
  name: %s
  owner: %s
spec:
  image: %s
  port: %d
  replicas: %d
  routing:
    domain: %s
    pathPrefix: %s
  networking:
    exposeToPlatform: %v
`

// GenerateApp creates a skeleton application manifest YAML file in the given directory.
func GenerateApp(opts AppOptions) error {
	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return fmt.Errorf("creating directory %q: %w", opts.OutputDir, err)
	}

	path := filepath.Join(opts.OutputDir, opts.Name+".yml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("application manifest already exists at %q", path)
	}

	content := fmt.Sprintf(appSkeleton,
		opts.Name,
		opts.Team,
		opts.Image,
		opts.Port,
		opts.Replicas,
		opts.Domain,
		opts.PathPrefix,
		opts.ExposeToPlatform,
	)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing application manifest: %w", err)
	}

	fmt.Printf("Created application manifest: %s\n", path)
	return nil
}
