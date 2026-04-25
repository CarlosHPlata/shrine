package state

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

type Deployment struct {
	Kind        string
	Name        string
	ContainerID string
	ConfigHash  string
}

type DeploymentStore interface {
	Record(team string, deployment Deployment) error
	Remove(team string, name string) error
	List(team string) ([]Deployment, error)
}

// ConfigHash produces a stable sha256 fingerprint of the container config.
// volSpecs must be "name:mountPath" strings; env must be "KEY=VALUE" strings.
// Both slices are sorted internally so call order does not matter.
func ConfigHash(image string, env []string, volSpecs []string, exposeToPlatform bool) string {
	sortedEnv := make([]string, len(env))
	copy(sortedEnv, env)
	sort.Strings(sortedEnv)

	sortedVols := make([]string, len(volSpecs))
	copy(sortedVols, volSpecs)
	sort.Strings(sortedVols)

	platform := "false"
	if exposeToPlatform {
		platform = "true"
	}

	input := strings.Join([]string{
		image,
		strings.Join(sortedEnv, "\n"),
		strings.Join(sortedVols, "\n"),
		platform,
	}, "\n---\n")

	sum := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", sum)
}
