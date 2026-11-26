package state

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Deployment struct {
	Kind        string
	Name        string
	ContainerID string
	ConfigHash  string
	Image       string // the reference the manifest named, as written, e.g. reg:lab/hello-api:1.2.0
	Policy      string // effective image pull policy at deploy time
}

type DeploymentStore interface {
	Record(team string, deployment Deployment) error
	Remove(team string, name string) error
	List(team string) ([]Deployment, error)
}

// ConfigHash produces a stable sha256 fingerprint of the container config.
// volSpecs must be "name:mountPath" strings; env must be "KEY=VALUE" strings;
// portSpecs must be "<hostPort>:<containerPort>/<proto>" strings.
// All slices are sorted internally so call order does not matter, except
// command, whose order is meaningful. command joins the input only when set,
// so hashes recorded before it existed stay valid.
func ConfigHash(image string, env, volSpecs, portSpecs []string, exposeToPlatform bool, command ...string) string {
	sortedEnv := make([]string, len(env))
	copy(sortedEnv, env)
	sort.Strings(sortedEnv)

	sortedVols := make([]string, len(volSpecs))
	copy(sortedVols, volSpecs)
	sort.Strings(sortedVols)

	sortedPorts := make([]string, len(portSpecs))
	copy(sortedPorts, portSpecs)
	sort.Strings(sortedPorts)

	platform := "false"
	if exposeToPlatform {
		platform = "true"
	}

	sections := []string{
		image,
		strings.Join(sortedEnv, "\n"),
		strings.Join(sortedVols, "\n"),
		strings.Join(sortedPorts, "\n"),
		platform,
	}
	if len(command) > 0 {
		// JSON keeps item boundaries unambiguous: ["a b"] != ["a", "b"].
		encoded, _ := json.Marshal(command)
		sections = append(sections, string(encoded))
	}
	input := strings.Join(sections, "\n---\n")

	sum := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", sum)
}
