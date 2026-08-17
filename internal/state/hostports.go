package state

import "errors"

var ErrHostPortNotFound = errors.New("host port allocation not found")
var ErrNoAvailableHostPorts = errors.New("no available host ports in the automatic range")
var ErrHostPortTaken = errors.New("host port already allocated")

// HostPortMap maps "team/app" keys to their published host port.
type HostPortMap map[string]int

// HostPortKey returns the canonical "team/app" allocation key.
func HostPortKey(team, app string) string {
	return team + "/" + app
}

// HostPortStore persists host-port allocations for published applications.
// Entries outlive containers and teardown; only explicit application or team
// deletion releases them, which is what keeps automatic ports stable across
// redeploys.
type HostPortStore interface {
	AllocateHostPort(team, app string) (int, error)
	ClaimHostPort(team, app string, port int) error
	GetHostPort(team, app string) (int, error)
	ReleaseHostPort(team, app string) error
	ReleaseTeamHostPorts(team string) error
	ListHostPorts() (HostPortMap, error)
}
