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

	env, err := NewLiveResolver(nil).ResolveApplication(app, ResolvedDependencies{Resources: resolved})
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
	_, err := NewLiveResolver(nil).ResolveApplication(app, ResolvedDependencies{Resources: map[string]map[string]string{}})
	if err == nil || !strings.Contains(err.Error(), "unknown resource") {
		t.Errorf("expected unknown resource error, got: %v", err)
	}
}
func TestResolveApplication_Templates(t *testing.T) {
	t.Run("simple template", func(t *testing.T) {
		app := &manifest.ApplicationManifest{
			Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
			Spec: manifest.ApplicationSpec{
				Env: []manifest.EnvVar{
					{Name: "FOO", Value: "hello"},
					{Name: "BAR", Template: "{{.FOO}}/world"},
				},
			},
		}
		env, err := NewLiveResolver(nil).ResolveApplication(app, ResolvedDependencies{})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if env["BAR"] != "hello/world" {
			t.Errorf("BAR = %q, want hello/world", env["BAR"])
		}
	})

	t.Run("template with valueFrom", func(t *testing.T) {
		app := &manifest.ApplicationManifest{
			Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
			Spec: manifest.ApplicationSpec{
				Env: []manifest.EnvVar{
					{Name: "HOST", ValueFrom: "resource.db.host"},
					{Name: "URL", Template: "http://{{.HOST}}:8080"},
				},
			},
		}
		resolved := map[string]map[string]string{
			"db": {"host": "db-server"},
		}
		env, err := NewLiveResolver(nil).ResolveApplication(app, ResolvedDependencies{Resources: resolved})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if env["URL"] != "http://db-server:8080" {
			t.Errorf("URL = %q, want http://db-server:8080", env["URL"])
		}
	})

	t.Run("template chain", func(t *testing.T) {
		app := &manifest.ApplicationManifest{
			Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
			Spec: manifest.ApplicationSpec{
				Env: []manifest.EnvVar{
					{Name: "C", Value: "C"},
					{Name: "B", Template: "{{.C}}+B"},
					{Name: "A", Template: "{{.B}}+A"},
				},
			},
		}
		env, err := NewLiveResolver(nil).ResolveApplication(app, ResolvedDependencies{})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if env["A"] != "C+B+A" {
			t.Errorf("A = %q, want C+B+A", env["A"])
		}
	})

	t.Run("cycle detected", func(t *testing.T) {
		app := &manifest.ApplicationManifest{
			Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
			Spec: manifest.ApplicationSpec{
				Env: []manifest.EnvVar{
					{Name: "A", Template: "{{.B}}"},
					{Name: "B", Template: "{{.A}}"},
				},
			},
		}
		_, err := NewLiveResolver(nil).ResolveApplication(app, ResolvedDependencies{})
		if err == nil || !strings.Contains(err.Error(), "template cycle") {
			t.Errorf("expected cycle error, got: %v", err)
		}
	})

	t.Run("built-ins visible", func(t *testing.T) {
		app := &manifest.ApplicationManifest{
			Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
			Spec: manifest.ApplicationSpec{
				Env: []manifest.EnvVar{
					{Name: "ID", Template: "{{.team}}.{{.name}}"},
				},
			},
		}
		env, err := NewLiveResolver(nil).ResolveApplication(app, ResolvedDependencies{})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if env["ID"] != "team-a.api" {
			t.Errorf("ID = %q, want team-a.api", env["ID"])
		}
	})

	t.Run("app-to-app dependency", func(t *testing.T) {
		app := &manifest.ApplicationManifest{
			Metadata: manifest.Metadata{Name: "consumer", Owner: "team-a"},
			Spec: manifest.ApplicationSpec{
				Env: []manifest.EnvVar{
					{Name: "PRODUCER_HOST", ValueFrom: "application.producer.host"},
				},
			},
		}
		resolvedApps := map[string]map[string]string{
			"producer": {"host": "team-b.producer", "port": "80"},
		}
		env, err := NewLiveResolver(nil).ResolveApplication(app, ResolvedDependencies{Applications: resolvedApps})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if env["PRODUCER_HOST"] != "team-b.producer" {
			t.Errorf("PRODUCER_HOST = %q, want team-b.producer", env["PRODUCER_HOST"])
		}
	})
}

func TestResolveApplication_AppBuiltins(t *testing.T) {
	app := &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Env: []manifest.EnvVar{
				{Name: "WORKER_HOST", ValueFrom: "application.worker.host"},
				{Name: "WORKER_PORT", ValueFrom: "application.worker.port"},
			},
		},
	}

	deps := ResolvedDependencies{
		Applications: map[string]map[string]string{
			"worker": {"host": "team-b.worker", "port": "8080"},
		},
	}

	env, err := NewLiveResolver(nil).ResolveApplication(app, deps)
	if err != nil {
		t.Fatalf("ResolveApplication returned error: %v", err)
	}

	if env["WORKER_HOST"] != "team-b.worker" {
		t.Errorf("WORKER_HOST = %q, want %q", env["WORKER_HOST"], "team-b.worker")
	}
	if env["WORKER_PORT"] != "8080" {
		t.Errorf("WORKER_PORT = %q, want %q", env["WORKER_PORT"], "8080")
	}

	t.Run("invalid built-in fails", func(t *testing.T) {
		badApp := &manifest.ApplicationManifest{
			Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
			Spec: manifest.ApplicationSpec{
				Env: []manifest.EnvVar{
					{Name: "URL", ValueFrom: "application.worker.url"},
				},
			},
		}
		_, err := NewLiveResolver(nil).ResolveApplication(badApp, deps)
		if err == nil || !strings.Contains(err.Error(), "has no resolved output \"url\"") {
			t.Errorf("expected missing output error, got: %v", err)
		}
	})
}
