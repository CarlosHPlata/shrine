package planner

import (
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

func makePublishingApp(owner, name string, hostPort int) *manifest.ApplicationManifest {
	return &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Owner: owner, Name: name},
		Spec: manifest.ApplicationSpec{
			Image:      "img",
			Port:       80,
			Networking: manifest.Networking{Publish: &manifest.Publish{HostPort: hostPort}},
		},
	}
}

func TestDetectHostPortCollisions_NoPublishersIsClean(t *testing.T) {
	set := setWith(
		makeApp("team-a", "app1", "a.example.com", ""),
	)
	if err := DetectHostPortCollisions(set, PortContext{}); err != nil {
		t.Errorf("expected nil for a set without publishers, got %v", err)
	}
}

func TestDetectHostPortCollisions_DistinctExplicitPortsAreClean(t *testing.T) {
	set := setWith(
		makePublishingApp("team-a", "app1", 8080),
		makePublishingApp("team-b", "app2", 9090),
	)
	if err := DetectHostPortCollisions(set, PortContext{}); err != nil {
		t.Errorf("expected nil for distinct ports, got %v", err)
	}
}

func TestDetectHostPortCollisions_AutomaticPublishersNeverConflict(t *testing.T) {
	set := setWith(
		makePublishingApp("team-a", "app1", 0),
		makePublishingApp("team-b", "app2", 0),
	)
	ports := PortContext{Reserved: []int{80}, Persisted: state.HostPortMap{"team-x/other": 30000}}
	if err := DetectHostPortCollisions(set, ports); err != nil {
		t.Errorf("automatic publishers must never be flagged, got %v", err)
	}
}

func TestDetectHostPortCollisions_DuplicateExplicit(t *testing.T) {
	set := setWith(
		makePublishingApp("team-b", "beta", 8080),
		makePublishingApp("team-a", "alpha", 8080),
	)
	err := DetectHostPortCollisions(set, PortContext{})
	if err == nil {
		t.Fatal("expected a duplicate-port error")
	}
	want := `host port collision: port 8080 declared by "team-a/alpha" and "team-b/beta"`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error should contain %q, got:\n%v", want, err)
	}
}

func TestDetectHostPortCollisions_ReservedPort(t *testing.T) {
	set := setWith(makePublishingApp("team-a", "edge", 8443))
	err := DetectHostPortCollisions(set, PortContext{Reserved: []int{8443}})
	if err == nil {
		t.Fatal("expected a reserved-port error")
	}
	want := `host port reserved: port 8443 declared by "team-a/edge" is reserved by the platform gateway`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error should contain %q, got:\n%v", want, err)
	}
}

func TestDetectHostPortCollisions_PersistedOtherApp(t *testing.T) {
	set := setWith(makePublishingApp("ops", "metrics", 8090))
	ports := PortContext{Persisted: state.HostPortMap{"media/photos": 8090}}
	err := DetectHostPortCollisions(set, ports)
	if err == nil {
		t.Fatal("expected a persisted-conflict error")
	}
	want := `host port taken: port 8090 declared by "ops/metrics" is already allocated to "media/photos"`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error should contain %q, got:\n%v", want, err)
	}
}

func TestDetectHostPortCollisions_SelfAdoptionIsClean(t *testing.T) {
	set := setWith(makePublishingApp("ops", "metrics", 8090))
	ports := PortContext{Persisted: state.HostPortMap{"ops/metrics": 8090}}
	if err := DetectHostPortCollisions(set, ports); err != nil {
		t.Errorf("an app claiming its own persisted port must pass, got %v", err)
	}
}

func TestDetectHostPortCollisions_AllConflictsInOneReport(t *testing.T) {
	set := setWith(
		makePublishingApp("team-a", "alpha", 8080),
		makePublishingApp("team-b", "beta", 8080),
		makePublishingApp("team-c", "gamma", 8443),
		makePublishingApp("team-d", "delta", 8090),
	)
	ports := PortContext{
		Reserved:  []int{8443},
		Persisted: state.HostPortMap{"media/photos": 8090},
	}
	err := DetectHostPortCollisions(set, ports)
	if err == nil {
		t.Fatal("expected errors")
	}
	msg := err.Error()
	for _, want := range []string{
		"host port collision: port 8080",
		"host port reserved: port 8443",
		"host port taken: port 8090",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("single invocation should report %q, got:\n%s", want, msg)
		}
	}
	if !strings.HasPrefix(msg, "host port validation failed:") {
		t.Errorf("aggregate error should start with the standard prefix, got:\n%s", msg)
	}
}

func TestDetectHostPortCollisions_Deterministic(t *testing.T) {
	set := setWith(
		makePublishingApp("team-b", "beta", 8080),
		makePublishingApp("team-a", "alpha", 8080),
		makePublishingApp("team-c", "gamma", 9090),
		makePublishingApp("team-d", "delta", 9090),
	)
	first := DetectHostPortCollisions(set, PortContext{}).Error()
	for i := 0; i < 10; i++ {
		if got := DetectHostPortCollisions(set, PortContext{}).Error(); got != first {
			t.Fatalf("non-deterministic output:\nfirst: %s\ngot:   %s", first, got)
		}
	}
}
