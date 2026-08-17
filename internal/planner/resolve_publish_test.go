package planner

import (
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// TestResolve_PublishGrantsNoCrossTeamAccess pins the deliberate decoupling:
// publishing implies platform-network attachment for localhost access but
// must never satisfy the cross-team reachability gate, which keeps reading
// the raw exposeToPlatform field.
func TestResolve_PublishGrantsNoCrossTeamAccess(t *testing.T) {
	set := NewManifestSet()
	set.Applications["target"] = &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "target", Owner: "team-a", Access: []string{"team-b"}},
		Spec: manifest.ApplicationSpec{
			Image:      "img",
			Port:       80,
			Networking: manifest.Networking{Publish: &manifest.Publish{HostPort: 8080}},
		},
	}
	set.Applications["consumer"] = &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "consumer", Owner: "team-b"},
		Spec: manifest.ApplicationSpec{
			Image: "img",
			Port:  80,
			Dependencies: []manifest.Dependency{
				{Kind: manifest.ApplicationKind, Name: "target", Owner: "team-a"},
			},
		},
	}

	errs := Resolve(set, stubTeamStore{}, nil)
	if len(errs) == 0 {
		t.Fatal("cross-team dependency on a publish-only app must be rejected")
	}
	joined := make([]string, len(errs))
	for i, err := range errs {
		joined[i] = err.Error()
	}
	all := strings.Join(joined, "\n")
	if !strings.Contains(all, "not reachable cross-team") {
		t.Errorf("expected the reachability gate to fire, got:\n%s", all)
	}

	// Flipping the raw field on makes the same dependency valid.
	set.Applications["target"].Spec.Networking.ExposeToPlatform = true
	if errs := Resolve(set, stubTeamStore{}, nil); len(errs) != 0 {
		t.Errorf("exposeToPlatform target should be reachable, got: %v", errs)
	}
}
