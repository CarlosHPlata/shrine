package handler

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/CarlosHPlata/shrine/internal/manifest"
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
	Image            string // as typed; empty means the default for PullPolicy
	PullPolicy       string // the configuration's imagePullPolicy, empty when unset
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

	if err := os.WriteFile(path, []byte(renderAppSkeleton(opts)), 0644); err != nil {
		return fmt.Errorf("writing application manifest: %w", err)
	}

	fmt.Printf("Created application manifest: %s\n", path)
	return nil
}

func renderAppSkeleton(opts AppOptions) string {
	image := opts.Image
	if image == "" {
		image = defaultAppImage(opts.Name, opts.PullPolicy)
	}
	return fmt.Sprintf(appSkeleton,
		opts.Name,
		opts.Team,
		image,
		opts.Port,
		opts.Replicas,
		opts.Domain,
		opts.PathPrefix,
		opts.ExposeToPlatform,
	)
}

// defaultAppImage names only the repository under Pinned, where a tag would
// be a fixed version the next deploy rejects (R-08).
func defaultAppImage(name, pullPolicy string) string {
	if pullPolicy == manifest.ImagePullPolicyPinned {
		return name
	}
	return name + ":latest"
}
