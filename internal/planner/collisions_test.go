package planner

import (
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

func makeApp(owner, name, domain, pathPrefix string, aliases ...manifest.RoutingAlias) *manifest.ApplicationManifest {
	return &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Owner: owner, Name: name},
		Spec: manifest.ApplicationSpec{
			Routing: manifest.Routing{
				Domain:     domain,
				PathPrefix: pathPrefix,
				Aliases:    aliases,
			},
		},
	}
}

func setWith(apps ...*manifest.ApplicationManifest) *ManifestSet {
	s := &ManifestSet{
		Applications: make(map[string]*manifest.ApplicationManifest),
		Resources:    make(map[string]*manifest.ResourceManifest),
	}
	for _, a := range apps {
		s.Applications[a.Metadata.Name] = a
	}
	return s
}

func TestDetectRoutingCollisions_Disjoint(t *testing.T) {
	set := setWith(
		makeApp("team-a", "app1", "a.example.com", ""),
		makeApp("team-b", "app2", "b.example.com", ""),
	)
	if err := DetectRoutingCollisions(set, NoFilter()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestDetectRoutingCollisions_PrimaryVsPrimary(t *testing.T) {
	set := setWith(
		makeApp("team-a", "app1", "clash.example.com", ""),
		makeApp("team-b", "app2", "clash.example.com", ""),
	)
	err := DetectRoutingCollisions(set, NoFilter())
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), "routing collision") {
		t.Errorf("expected 'routing collision' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "team-a/app1") || !strings.Contains(err.Error(), "team-b/app2") {
		t.Errorf("expected both app refs in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "clash.example.com") {
		t.Errorf("expected host in error, got: %v", err)
	}
}

func TestDetectRoutingCollisions_PrimaryVsAlias(t *testing.T) {
	alias := manifest.RoutingAlias{Host: "shared.example.com", PathPrefix: "/api"}
	set := setWith(
		makeApp("team-a", "app1", "shared.example.com", "/api"),
		makeApp("team-b", "app2", "other.example.com", "", alias),
	)
	err := DetectRoutingCollisions(set, NoFilter())
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), "routing collision") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDetectRoutingCollisions_AliasVsAlias(t *testing.T) {
	alias1 := manifest.RoutingAlias{Host: "shared.example.com", PathPrefix: "/x"}
	alias2 := manifest.RoutingAlias{Host: "shared.example.com", PathPrefix: "/x"}
	set := setWith(
		makeApp("team-a", "app1", "a.example.com", "", alias1),
		makeApp("team-b", "app2", "b.example.com", "", alias2),
	)
	err := DetectRoutingCollisions(set, NoFilter())
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), "routing collision") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDetectRoutingCollisions_MultipleCollisions(t *testing.T) {
	alias := manifest.RoutingAlias{Host: "extra.example.com", PathPrefix: ""}
	set := setWith(
		makeApp("team-a", "app1", "clash1.example.com", "", alias),
		makeApp("team-b", "app2", "clash1.example.com", "", alias),
	)
	err := DetectRoutingCollisions(set, NoFilter())
	if err == nil {
		t.Fatal("expected collision error")
	}
	// Two collisions: primary-vs-primary and alias-vs-alias
	if count := strings.Count(err.Error(), "routing collision"); count < 2 {
		t.Errorf("expected at least 2 collision lines, got %d in: %v", count, err)
	}
}

func TestDetectRoutingCollisions_TrailingSlashNormalization(t *testing.T) {
	alias := manifest.RoutingAlias{Host: "shared.example.com", PathPrefix: "/x/"}
	set := setWith(
		makeApp("team-a", "app1", "shared.example.com", "/x"),
		makeApp("team-b", "app2", "other.example.com", "", alias),
	)
	err := DetectRoutingCollisions(set, NoFilter())
	if err == nil {
		t.Fatal("expected collision error for /x vs /x/ (normalized equal)")
	}
	if !strings.Contains(err.Error(), "routing collision") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDetectRoutingCollisions_SameApp_NoDuplicate(t *testing.T) {
	// Within one app the same host+path appears twice — but the validator
	// catches this; the collision detector must not report it.
	alias := manifest.RoutingAlias{Host: "a.example.com", PathPrefix: ""}
	set := setWith(
		makeApp("team-a", "app1", "a.example.com", "", alias),
	)
	if err := DetectRoutingCollisions(set, NoFilter()); err != nil {
		t.Errorf("expected nil for same-app duplicate, got %v", err)
	}
}

func TestDetectRoutingCollisions_TeamScope_IgnoresOutOfScopePair(t *testing.T) {
	set := setWith(
		makeApp("team-a", "alpha", "a.example.com", ""),
		makeApp("team-b", "beta1", "clash.example.com", ""),
		makeApp("team-b", "beta2", "clash.example.com", ""),
	)
	if err := DetectRoutingCollisions(set, ByTeam("team-a")); err != nil {
		t.Errorf("expected nil for a collision entirely outside team-a, got %v", err)
	}
}

func TestDetectRoutingCollisions_TeamScope_ReportsInScopeVsOutOfScope(t *testing.T) {
	set := setWith(
		makeApp("team-a", "alpha", "clash.example.com", ""),
		makeApp("team-b", "beta", "clash.example.com", ""),
	)
	err := DetectRoutingCollisions(set, ByTeam("team-a"))
	if err == nil {
		t.Fatal("expected collision error")
	}
	for _, want := range []string{"team-a/alpha", "team-b/beta", "clash.example.com"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected %q in error, got: %v", want, err)
		}
	}
}

func TestDetectRoutingCollisions_AppScope_ReportsOnlyPairsTouchingApp(t *testing.T) {
	set := setWith(
		makeApp("team-a", "alpha", "clash.example.com", ""),
		makeApp("team-b", "beta", "clash.example.com", ""),
		makeApp("team-c", "gamma", "other.example.com", ""),
		makeApp("team-d", "delta", "other.example.com", ""),
	)
	err := DetectRoutingCollisions(set, ByApp("alpha"))
	if err == nil {
		t.Fatal("expected collision error")
	}
	if count := strings.Count(err.Error(), "routing collision"); count != 1 {
		t.Errorf("expected exactly 1 collision line, got %d in: %v", count, err)
	}
	if !strings.Contains(err.Error(), "team-a/alpha") || !strings.Contains(err.Error(), "team-b/beta") {
		t.Errorf("expected alpha and beta in error, got: %v", err)
	}
	if strings.Contains(err.Error(), "gamma") || strings.Contains(err.Error(), "delta") {
		t.Errorf("out-of-scope pair must not be reported, got: %v", err)
	}
}

func TestDetectRoutingCollisions_ResScope_AlwaysNil(t *testing.T) {
	set := setWith(
		makeApp("team-a", "alpha", "clash.example.com", ""),
		makeApp("team-b", "beta", "clash.example.com", ""),
	)
	if err := DetectRoutingCollisions(set, ByResource("db")); err != nil {
		t.Errorf("expected nil under resource scope, got %v", err)
	}
}

func TestDetectRoutingCollisions_ThreeClaimants_OutOfScopeFirstStillPairsWithInScope(t *testing.T) {
	set := setWith(
		makeApp("team-a", "aaa", "clash.example.com", ""),
		makeApp("team-b", "bbb", "clash.example.com", ""),
		makeApp("team-b", "ccc", "clash.example.com", ""),
	)
	err := DetectRoutingCollisions(set, ByTeam("team-b"))
	if err == nil {
		t.Fatal("expected collision error")
	}
	if count := strings.Count(err.Error(), "routing collision"); count != 2 {
		t.Errorf("expected 2 collision lines, got %d in: %v", count, err)
	}
	for _, want := range []string{"team-a/aaa", "team-b/bbb", "team-b/ccc"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected %q in error, got: %v", want, err)
		}
	}
}
