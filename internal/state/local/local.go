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

	return &state.Store{
		Teams: teams,
	}, nil
}
