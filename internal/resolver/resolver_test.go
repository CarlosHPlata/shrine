package resolver

import (
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// fakeSecrets is a minimal in-memory implementation of state.SecretStore
// sufficient for exercising GetOrGenerate in these tests.
type fakeSecrets struct {
	data map[string]map[string]string
}

func newFakeSecrets() *fakeSecrets {
	return &fakeSecrets{data: map[string]map[string]string{}}
}

func (f *fakeSecrets) GetOrGenerate(team, key string, length int) (string, bool, error) {
	if f.data[team] == nil {
		f.data[team] = map[string]string{}
	}
	if v, ok := f.data[team][key]; ok {
		return v, false, nil
	}
	v := "secret-" + key
	f.data[team][key] = v
	return v, true, nil
}

func (f *fakeSecrets) Get(team, key string) (string, error) {
	return f.data[team][key], nil
}

func (f *fakeSecrets) List(team string) (map[string]string, error) {
	return f.data[team], nil
}

func TestResolveResource_MixedOutputs(t *testing.T) {
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "hello-db", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Outputs: []manifest.Output{
				{Name: "host"},
				{Name: "port", Value: "5432"},
				{Name: "database", Value: "hello"},
				{Name: "password", Generated: true},
				{Name: "url", Template: "postgres://postgres:{{.password}}@{{.host}}:{{.port}}/{{.database}}"},
			},
		},
	}

	r := NewLiveResolver(newFakeSecrets())
	values, err := r.ResolveResource(res)
	if err != nil {
		t.Fatalf("ResolveResource returned error: %v", err)
	}

	if values["host"] != "team-a.hello-db" {
		t.Errorf("host = %q, want %q", values["host"], "team-a.hello-db")
	}
	if values["port"] != "5432" {
		t.Errorf("port = %q, want %q", values["port"], "5432")
	}
	if values["password"] == "" {
		t.Errorf("password is empty, want generated value")
	}
	wantURL := "postgres://postgres:" + values["password"] + "@team-a.hello-db:5432/hello"
	if values["url"] != wantURL {
		t.Errorf("url = %q, want %q", values["url"], wantURL)
	}
}

func TestResolveResource_TemplateChain(t *testing.T) {
	// a depends on b, b depends on c — topological order must resolve all.
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "chain", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Outputs: []manifest.Output{
				{Name: "c", Value: "C"},
				{Name: "b", Template: "{{.c}}+B"},
				{Name: "a", Template: "{{.b}}+A"},
			},
		},
	}

	r := NewLiveResolver(newFakeSecrets())
	values, err := r.ResolveResource(res)
	if err != nil {
		t.Fatalf("ResolveResource returned error: %v", err)
	}
	if values["a"] != "C+B+A" {
		t.Errorf("a = %q, want %q", values["a"], "C+B+A")
	}
}

func TestResolveResource_TemplateCycle(t *testing.T) {
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "cycle", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Outputs: []manifest.Output{
				{Name: "a", Template: "{{.b}}"},
				{Name: "b", Template: "{{.a}}"},
			},
		},
	}

	r := NewLiveResolver(newFakeSecrets())
	_, err := r.ResolveResource(res)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "template cycle") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

func TestResolveResource_BareNonHostFails(t *testing.T) {
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "db", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Outputs: []manifest.Output{
				{Name: "mystery"},
			},
		},
	}
	_, err := NewLiveResolver(newFakeSecrets()).ResolveResource(res)
	if err == nil || !strings.Contains(err.Error(), "bare output") {
		t.Errorf("expected bare-output error, got: %v", err)
	}
}

func TestResolveApplication_StaticAndValueFrom(t *testing.T) {
	app := &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Env: []manifest.EnvVar{
				{Name: "NODE_ENV", Value: "production"},
				{Name: "DATABASE_URL", ValueFrom: "resource.db.url"},
			},
		},
	}

	resolved := map[string]map[string]string{
		"db": {"url": "postgres://..."},
	}

	env, err := NewLiveResolver(nil).ResolveApplication(app, resolved)
	if err != nil {
		t.Fatalf("ResolveApplication returned error: %v", err)
	}
	if env["NODE_ENV"] != "production" {
		t.Errorf("NODE_ENV = %q, want production", env["NODE_ENV"])
	}
	if env["DATABASE_URL"] != "postgres://..." {
		t.Errorf("DATABASE_URL = %q, want %q", env["DATABASE_URL"], "postgres://...")
	}
}

func TestResolveApplication_MissingResource(t *testing.T) {
	app := &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Env: []manifest.EnvVar{
				{Name: "X", ValueFrom: "resource.missing.url"},
			},
		},
	}
	_, err := NewLiveResolver(nil).ResolveApplication(app, map[string]map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "unknown resource") {
		t.Errorf("expected unknown resource error, got: %v", err)
	}
}
