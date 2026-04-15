package manifest

import (
	"path/filepath"
	"runtime"
	"testing"
)

func testdataPath(name string) string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "..", "..", "test", "testdata", name)
}

func TestParse_ApplicationManifest(t *testing.T) {
	m, err := Parse(testdataPath("hello-api.yml"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if m.Kind != "Application" {
		t.Errorf("Kind = %q, want %q", m.Kind, "Application")
	}
	if m.APIVersion != "shrine/v1" {
		t.Errorf("APIVersion = %q, want %q", m.APIVersion, "shrine/v1")
	}
	if m.Application == nil {
		t.Fatalf("Application is nil, expected parsed ApplicationManifest")
	}

	app := m.Application

	if app.Metadata.Name != "hello-api" {
		t.Errorf("Metadata.Name = %q, want %q", app.Metadata.Name, "hello-api")
	}
	if app.Metadata.Owner != "team-a" {
		t.Errorf("Metadata.Owner = %q, want %q", app.Metadata.Owner, "team-a")
	}
	if app.Spec.Image != "hello-api" {
		t.Errorf("Spec.Image = %q, want %q", app.Spec.Image, "hello-api")
	}
	if app.Spec.Port != 8080 {
		t.Errorf("Spec.Port = %d, want %d", app.Spec.Port, 8080)
	}
	if app.Spec.Replicas != 1 {
		t.Errorf("Spec.Replicas = %d, want %d", app.Spec.Replicas, 1)
	}
	if app.Spec.Routing.Domain != "hello-api.home.lab" {
		t.Errorf("Routing.Domain = %q, want %q", app.Spec.Routing.Domain, "hello-api.home.lab")
	}
	if app.Spec.Routing.PathPrefix != "/hello-api" {
		t.Errorf("Routing.PathPrefix = %q, want %q", app.Spec.Routing.PathPrefix, "/hello-api")
	}

	if len(app.Spec.Dependencies) != 1 {
		t.Fatalf("Dependencies count = %d, want 1", len(app.Spec.Dependencies))
	}
	dep := app.Spec.Dependencies[0]
	if dep.Kind != "Resource" {
		t.Errorf("Dependency.Kind = %q, want %q", dep.Kind, "Resource")
	}
	if dep.Name != "hello-db" {
		t.Errorf("Dependency.Name = %q, want %q", dep.Name, "hello-db")
	}
	if dep.Owner != "team-a" {
		t.Errorf("Dependency.Owner = %q, want %q", dep.Owner, "team-a")
	}

	if len(app.Spec.Env) != 2 {
		t.Fatalf("Env count = %d, want 2", len(app.Spec.Env))
	}
	if app.Spec.Env[0].Name != "DATABASE_URL" {
		t.Errorf("Env[0].Name = %q, want %q", app.Spec.Env[0].Name, "DATABASE_URL")
	}
	if app.Spec.Env[0].ValueFrom != "dependency.hello-db.url" {
		t.Errorf("Env[0].ValueFrom = %q, want %q", app.Spec.Env[0].ValueFrom, "dependency.hello-db.url")
	}
	if app.Spec.Env[1].Name != "NODE_ENV" {
		t.Errorf("Env[1].Name = %q, want %q", app.Spec.Env[1].Name, "NODE_ENV")
	}
	if app.Spec.Env[1].Value != "production" {
		t.Errorf("Env[1].Value = %q, want %q", app.Spec.Env[1].Value, "production")
	}
}

func TestParse_TeamManifest(t *testing.T) {
	m, err := Parse(testdataPath("hello-team.yml"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if m.Kind != "Team" {
		t.Errorf("Kind = %q, want %q", m.Kind, "Team")
	}
	if m.APIVersion != "shrine/v1" {
		t.Errorf("APIVersion = %q, want %q", m.APIVersion, "shrine/v1")
	}
	if m.Team == nil {
		t.Fatalf("Team is nil, expected parsed TeamManifest")
	}

	team := m.Team

	if team.Metadata.Name != "team-a" {
		t.Errorf("Metadata.Name = %q, want %q", team.Metadata.Name, "team-a")
	}
	if team.Spec.DisplayName != "Team Alpha" {
		t.Errorf("Spec.DisplayName = %q, want %q", team.Spec.DisplayName, "Team Alpha")
	}
	if team.Spec.Contact != "alice@example.com" {
		t.Errorf("Spec.Contact = %q, want %q", team.Spec.Contact, "alice@example.com")
	}
	if team.Spec.RegistryUser != "alice" {
		t.Errorf("Spec.RegistryUser = %q, want %q", team.Spec.RegistryUser, "alice")
	}
	if team.Spec.Quotas.MaxApps != 3 {
		t.Errorf("Quotas.MaxApps = %d, want %d", team.Spec.Quotas.MaxApps, 3)
	}
	if team.Spec.Quotas.MaxResources != 5 {
		t.Errorf("Quotas.MaxResources = %d, want %d", team.Spec.Quotas.MaxResources, 5)
	}
	if len(team.Spec.Quotas.AllowedResourceTypes) != 2 {
		t.Fatalf("AllowedResourceTypes count = %d, want 2", len(team.Spec.Quotas.AllowedResourceTypes))
	}
	if team.Spec.Quotas.AllowedResourceTypes[0] != "postgres" {
		t.Errorf("AllowedResourceTypes[0] = %q, want %q", team.Spec.Quotas.AllowedResourceTypes[0], "postgres")
	}
	if team.Spec.Quotas.AllowedResourceTypes[1] != "rabbitmq" {
		t.Errorf("AllowedResourceTypes[1] = %q, want %q", team.Spec.Quotas.AllowedResourceTypes[1], "rabbitmq")
	}
}

func TestParse_ResourceManifest(t *testing.T) {
	m, err := Parse(testdataPath("hello-db.yml"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if m.Kind != "Resource" {
		t.Errorf("Kind = %q, want %q", m.Kind, "Resource")
	}
	if m.APIVersion != "shrine/v1" {
		t.Errorf("APIVersion = %q, want %q", m.APIVersion, "shrine/v1")
	}
	if m.Resource == nil {
		t.Fatalf("Resource is nil, expected parsed ResourceManifest")
	}

	res := m.Resource

	if res.Metadata.Name != "hello-db" {
		t.Errorf("Metadata.Name = %q, want %q", res.Metadata.Name, "hello-db")
	}
	if res.Metadata.Owner != "team-a" {
		t.Errorf("Metadata.Owner = %q, want %q", res.Metadata.Owner, "team-a")
	}
	if len(res.Metadata.Access) != 1 || res.Metadata.Access[0] != "team-b" {
		t.Errorf("Metadata.Access = %v, want [team-b]", res.Metadata.Access)
	}
	if res.Spec.Type != "postgres" {
		t.Errorf("Spec.Type = %q, want %q", res.Spec.Type, "postgres")
	}
	if res.Spec.Version != "16" {
		t.Errorf("Spec.Version = %q, want %q", res.Spec.Version, "16")
	}
}
