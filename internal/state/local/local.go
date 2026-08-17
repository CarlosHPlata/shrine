package local

import (
	"github.com/CarlosHPlata/shrine/internal/state"
)

// NewLocalStore initializes all filesystem-based stores and returns an
// aggregate Store. reservedHostPorts seeds the host-port allocator with the
// ports the platform gateway occupies.
func NewLocalStore(baseDir string, reservedHostPorts []int) (*state.Store, error) {
	teams, err := NewTeamStore(baseDir)
	if err != nil {
		return nil, err
	}

	subnets, err := NewSubnetStore(baseDir)
	if err != nil {
		return nil, err
	}

	secrets, err := NewSecretStore(baseDir)
	if err != nil {
		return nil, err
	}

	deployments, err := NewDeploymentStore(baseDir)
	if err != nil {
		return nil, err
	}

	hostPorts, err := NewHostPortStore(baseDir, reservedHostPorts)
	if err != nil {
		return nil, err
	}

	return &state.Store{
		Teams:       teams,
		Subnets:     subnets,
		Secrets:     secrets,
		Deployments: deployments,
		HostPorts:   hostPorts,
	}, nil
}
