package local

import (
	"github.com/CarlosHPlata/shrine/internal/state"
)

// NewLocalStore initializes all filesystem-based stores and returns an aggregate Store.
func NewLocalStore(baseDir string) (*state.Store, error) {
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

	return &state.Store{
		Teams:   teams,
		Subnets: subnets,
		Secrets: secrets,
	}, nil
}
