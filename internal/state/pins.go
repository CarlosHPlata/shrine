package state

import (
	"errors"
	"time"
)

var ErrImagePinNotFound = errors.New("image pin not found")

// ImagePin is Shrine's record of a Shrine-owned version: the expanded tag
// reference that was resolved, the pullable exact version it resolved to,
// and when. Pins outlive containers and teardown; only delete and a deploy
// under a manifest-owned policy release them.
type ImagePin struct {
	Kind      string
	Name      string
	Requested string    // expanded tag reference: ghcr.io/me/hello-api:latest, postgres
	Pinned    string    // pullable exact version: ghcr.io/me/hello-api@sha256:…
	PinnedAt  time.Time // UTC
}

// ImagePinKey returns the canonical "team/name" key used by ListAll.
func ImagePinKey(team, name string) string {
	return team + "/" + name
}

// ImagePinStore persists image pins, one file per team.
type ImagePinStore interface {
	Get(team, name string) (ImagePin, error)
	Put(team string, pin ImagePin) error
	Release(team, name string) error
	ReleaseTeam(team string) error
	List(team string) ([]ImagePin, error)
	ListAll() (map[string]ImagePin, error)
}
