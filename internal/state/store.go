package state

import "github.com/CarlosHPlata/shrine/internal/manifest"

// Store defines the interface for persisting platform state.
// This allows us to swap the filesystem-based storage for a database or remote API later.
type Store interface {
	SaveTeam(team *manifest.TeamManifest) error
	LoadTeam(name string) (*manifest.TeamManifest, error)
	ListTeams() ([]*manifest.TeamManifest, error)
	DeleteTeam(name string) error
}
