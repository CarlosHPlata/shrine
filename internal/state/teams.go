package state

import "github.com/CarlosHPlata/shrine/internal/manifest"

// TeamStore defines the interface for persisting platform state related to teams.
// This allows us to swap the filesystem-based storage for a database or remote API later.
type TeamStore interface {
	SaveTeam(team *manifest.TeamManifest) error
	LoadTeam(name string) (*manifest.TeamManifest, error)
	ListTeams() ([]*manifest.TeamManifest, error)
	DeleteTeam(name string) error
}
