package handler

import (
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/planner"
)

func TestFormatDeployPlan_RendersHeaderAndOrdinals(t *testing.T) {
	set := planner.NewManifestSet()
	set.Resources["db"] = &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "db", Owner: "team-a"},
	}
	set.Applications["api"] = &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Dependencies: []manifest.Dependency{
				{Kind: manifest.ResourceKind, Name: "db", Owner: "team-a"},
			},
		},
	}

	steps := []planner.PlannedStep{
		{Kind: manifest.ResourceKind, Name: "db"},
		{Kind: manifest.ApplicationKind, Name: "api"},
	}
	edges := []planner.InferredEdge{{
		Consumer: planner.ManifestRef{Kind: manifest.ApplicationKind, Name: "api", Owner: "team-a"},
		Target:   planner.ManifestRef{Kind: manifest.ResourceKind, Name: "db", Owner: "team-a"},
		EnvVar:   "DB_URL",
	}}

	out := formatDeployPlan(steps, set, edges)
	if !strings.HasPrefix(out, "Deploy order:\n") {
		t.Errorf("missing header. got:\n%s", out)
	}
	if !strings.Contains(out, "  1. Resource:db\n") {
		t.Errorf("missing step 1. got:\n%s", out)
	}
	if !strings.Contains(out, "  2. Application:api\n") {
		t.Errorf("missing step 2. got:\n%s", out)
	}
	if !strings.Contains(out, "       depends on:\n         - Resource:db (inferred from env DB_URL)\n") {
		t.Errorf("missing inferred dep tag. got:\n%s", out)
	}
}

func TestFormatDeployPlan_ExplicitDepHasNoTag(t *testing.T) {
	set := planner.NewManifestSet()
	set.Applications["api"] = &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Dependencies: []manifest.Dependency{
				{Kind: manifest.ResourceKind, Name: "db", Owner: "team-a"},
			},
		},
	}
	steps := []planner.PlannedStep{
		{Kind: manifest.ApplicationKind, Name: "api"},
	}
	out := formatDeployPlan(steps, set, nil)
	if !strings.Contains(out, "         - Resource:db\n") {
		t.Errorf("expected untagged explicit dep. got:\n%s", out)
	}
	if strings.Contains(out, "inferred from env") {
		t.Errorf("explicit dep must not be tagged. got:\n%s", out)
	}
}

func TestFormatDeployPlan_NoDepsNoDependsOnBlock(t *testing.T) {
	set := planner.NewManifestSet()
	set.Resources["db"] = &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "db"},
	}
	steps := []planner.PlannedStep{{Kind: manifest.ResourceKind, Name: "db"}}
	out := formatDeployPlan(steps, set, nil)
	if strings.Contains(out, "depends on:") {
		t.Errorf("steps with no deps must omit block. got:\n%s", out)
	}
}
