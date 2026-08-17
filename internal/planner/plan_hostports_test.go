package planner

import (
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

// conflictedSet returns twoTeamSet with alpha explicitly claiming a port that
// another application's persisted allocation already holds.
func conflictedPortSet() (*ManifestSet, PortContext) {
	set := twoTeamSet()
	set.Applications["alpha"].Spec.Networking.Publish = &manifest.Publish{HostPort: 8080}
	ports := PortContext{Persisted: state.HostPortMap{"team-x/other": 8080}}
	return set, ports
}

func TestPlan_HostPortCollisions_CheckedForAllFilters(t *testing.T) {
	filters := map[string]Filter{
		"NoFilter":   NoFilter(),
		"ByTeam":     ByTeam("team-a"),
		"ByApp":      ByApp("alpha"),
		"ByResource": ByResource("db-a"),
	}
	for name, filter := range filters {
		t.Run(name, func(t *testing.T) {
			set, ports := conflictedPortSet()
			result := Plan(set, stubTeamStore{}, nil, ports, filter)
			if result.Error == nil {
				t.Fatalf("filter %s: expected a host-port conflict error, got none", name)
			}
			if !strings.Contains(result.Error.Error(), "host port") {
				t.Errorf("filter %s: error should mention host ports, got: %v", name, result.Error)
			}
		})
	}
}

func TestPlan_CleanPortsProduceNoError(t *testing.T) {
	set := twoTeamSet()
	set.Applications["alpha"].Spec.Networking.Publish = &manifest.Publish{HostPort: 8080}
	ports := PortContext{Reserved: []int{80}, Persisted: state.HostPortMap{"team-a/alpha": 8080}}

	result := Plan(set, stubTeamStore{}, nil, ports, NoFilter())
	if result.Error != nil {
		t.Fatalf("self-adopting plan should succeed, got: %v", result.Error)
	}
	if len(result.Steps) == 0 {
		t.Fatal("expected steps for a clean plan")
	}
}
