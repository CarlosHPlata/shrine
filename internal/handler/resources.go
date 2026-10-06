package handler

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

type ResourceOptions struct {
	Name             string
	Team             string
	OutputDir        string
	Type             string
	Version          string // as typed; empty means the default for PullPolicy
	ExposeToPlatform bool
	PullPolicy       string // the configuration's imagePullPolicy, empty when unset
}

const defaultResourceVersionTag = "16"

const resourceSkeleton = `apiVersion: shrine/v1
kind: Resource
metadata:
  name: %s
  owner: %s
spec:
  type: %s
%s  networking:
    exposeToPlatform: %v
  # env declares the container's runtime configuration (same shape as an
  # Application's env, plus generated secrets).
  # env:
  #   - name: POSTGRES_DB
  #     value: app
  #   - name: POSTGRES_PASSWORD
  #     generated: true          # auto-minted secret
  # outputs declares the export allowlist consumers may read: a name (re-exporting
  # an env var or the built-in host/port) plus an optional template. No values.
  # Outputs are export-only — they are NEVER set as this container's own env vars.
  # If the container itself needs a value, declare it under env (above).
  # outputs:
  #   - name: POSTGRES_DB        # re-export an env var
  #   - name: host               # built-in
  #   - name: DB_URL
  #     template: "postgres://app:{{.POSTGRES_PASSWORD}}@{{.host}}:{{.port}}/{{.POSTGRES_DB}}"
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

	if err := os.WriteFile(path, []byte(renderResourceSkeleton(opts)), 0644); err != nil {
		return fmt.Errorf("writing resource manifest: %w", err)
	}

	fmt.Printf("Created resource manifest: %s\n", path)
	return nil
}

func renderResourceSkeleton(opts ResourceOptions) string {
	version := opts.Version
	if version == "" {
		version = defaultResourceVersion(opts.PullPolicy)
	}
	return fmt.Sprintf(resourceSkeleton,
		opts.Name,
		opts.Team,
		opts.Type,
		versionLine(version),
		opts.ExposeToPlatform,
	)
}

// defaultResourceVersion is empty under Pinned so the skeleton omits the
// line: a version would be a fixed version the next deploy rejects (R-08).
func defaultResourceVersion(pullPolicy string) string {
	if pullPolicy == manifest.ImagePullPolicyPinned {
		return ""
	}
	return defaultResourceVersionTag
}

func versionLine(version string) string {
	if version == "" {
		return ""
	}
	return fmt.Sprintf("  version: %q\n", version)
}
