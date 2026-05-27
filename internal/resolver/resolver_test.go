package resolver

import (
	"errors"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// fakeSecrets is a minimal in-memory implementation of state.SecretStore.
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

// fakeVault implements secrets.SecretsPlugin for testing.
type fakeVault struct {
	data map[string]string
	err  error
}

func newFakeVault(kv ...string) *fakeVault {
	m := make(map[string]string)
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return &fakeVault{data: m}
}

func (v *fakeVault) IsActive() bool { return true }

func (v *fakeVault) GetSecret(path string) (string, error) {
	if v.err != nil {
		return "", v.err
	}
	val, ok := v.data[path]
	if !ok {
		return "", errors.New("vault: secret not found: " + path)
	}
	return val, nil
}

// nilVault simulates an unconfigured vault (IsActive returns false).
type nilVault struct{}

func (nilVault) IsActive() bool              { return false }
func (nilVault) GetSecret(_ string) (string, error) { return "", errors.New("inactive") }

func TestResolveResource_EnvAndExports(t *testing.T) {
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "pg", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Port: 5432,
			Env: []manifest.EnvVar{
				{Name: "POSTGRES_DB", Value: "app"},
				{Name: "POSTGRES_PASSWORD", Generated: true},
			},
			Outputs: []manifest.Output{
				{Name: "POSTGRES_DB"},
				{Name: "host"},
				{Name: "port"},
				{Name: "DB_URL", Template: "postgres://app:{{.POSTGRES_PASSWORD}}@{{.host}}:{{.port}}/{{.POSTGRES_DB}}"},
			},
		},
	}

	r := NewLiveResolver(newFakeSecrets(), nil)
	got, err := r.ResolveResource(res, ResolvedDependencies{})
	if err != nil {
		t.Fatalf("ResolveResource returned error: %v", err)
	}

	// Env feeds the container and holds every declared env var.
	if got.Env["POSTGRES_DB"] != "app" {
		t.Errorf("Env[POSTGRES_DB] = %q, want app", got.Env["POSTGRES_DB"])
	}
	if got.Env["POSTGRES_PASSWORD"] == "" {
		t.Error("Env[POSTGRES_PASSWORD] is empty, want generated value")
	}
	if _, ok := got.Env["DB_URL"]; ok {
		t.Error("DB_URL is an export, must not appear in container Env")
	}

	// Exports is the strict allowlist.
	if _, ok := got.Exports["POSTGRES_PASSWORD"]; ok {
		t.Error("POSTGRES_PASSWORD is not listed in output and must not be exported")
	}
	if got.Exports["host"] != "team-a.pg" {
		t.Errorf("Exports[host] = %q, want team-a.pg", got.Exports["host"])
	}
	if got.Exports["port"] != "5432" {
		t.Errorf("Exports[port] = %q, want 5432", got.Exports["port"])
	}
	wantURL := "postgres://app:" + got.Env["POSTGRES_PASSWORD"] + "@team-a.pg:5432/app"
	if got.Exports["DB_URL"] != wantURL {
		t.Errorf("Exports[DB_URL] = %q, want %q", got.Exports["DB_URL"], wantURL)
	}
}

func TestResolveResource_GeneratedKeyReuse(t *testing.T) {
	secrets := newFakeSecrets()
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "pg", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Env: []manifest.EnvVar{{Name: "PW", Generated: true}},
		},
	}
	r := NewLiveResolver(secrets, nil)
	first, err := r.ResolveResource(res, ResolvedDependencies{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := r.ResolveResource(res, ResolvedDependencies{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Env["PW"] != second.Env["PW"] {
		t.Error("generated secret must be stable for the same key across resolves")
	}
	if _, ok := secrets.data["team-a"]["pg.PW"]; !ok {
		t.Errorf("expected secret key %q, got store %v", "pg.PW", secrets.data)
	}
}

func TestResolveResource_VaultEnv(t *testing.T) {
	vault := newFakeVault("myproject/prod/DB_PASS", "s3cr3t")
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "db", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Env:     []manifest.EnvVar{{Name: "PASSWORD", ValueFrom: "vault:myproject/prod/DB_PASS"}},
			Outputs: []manifest.Output{{Name: "PASSWORD"}},
		},
	}

	r := NewLiveResolver(newFakeSecrets(), vault)
	got, err := r.ResolveResource(res, ResolvedDependencies{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Env["PASSWORD"] != "s3cr3t" {
		t.Errorf("Env[PASSWORD] = %q, want s3cr3t", got.Env["PASSWORD"])
	}
	if got.Exports["PASSWORD"] != "s3cr3t" {
		t.Errorf("Exports[PASSWORD] = %q, want s3cr3t", got.Exports["PASSWORD"])
	}
}

func TestResolveResource_VaultEnv_NoPlugin(t *testing.T) {
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "db", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Env: []manifest.EnvVar{{Name: "PASSWORD", ValueFrom: "vault:proj/prod/PASS"}},
		},
	}

	t.Run("nil vault", func(t *testing.T) {
		_, err := NewLiveResolver(newFakeSecrets(), nil).ResolveResource(res, ResolvedDependencies{})
		if err == nil || !strings.Contains(err.Error(), "no secrets plugin") {
			t.Errorf("expected no-plugin error, got: %v", err)
		}
	})

	t.Run("inactive vault", func(t *testing.T) {
		_, err := NewLiveResolver(newFakeSecrets(), nilVault{}).ResolveResource(res, ResolvedDependencies{})
		if err == nil || !strings.Contains(err.Error(), "no secrets plugin") {
			t.Errorf("expected no-plugin error, got: %v", err)
		}
	})
}

func TestResolveResource_VaultEnv_MissingSecret(t *testing.T) {
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "db", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Env: []manifest.EnvVar{{Name: "PASSWORD", ValueFrom: "vault:proj/prod/MISSING"}},
		},
	}

	_, err := NewLiveResolver(newFakeSecrets(), newFakeVault()).ResolveResource(res, ResolvedDependencies{})
	if err == nil {
		t.Fatal("expected error for missing secret, got nil")
	}
	if !strings.Contains(err.Error(), "proj/prod/MISSING") {
		t.Errorf("error should contain secret path, got: %v", err)
	}
}

func TestResolveResource_EnvTemplateChain(t *testing.T) {
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "chain", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Env: []manifest.EnvVar{
				{Name: "C", Value: "C"},
				{Name: "B", Template: "{{.C}}+B"},
				{Name: "A", Template: "{{.B}}+A"},
			},
		},
	}

	got, err := NewLiveResolver(newFakeSecrets(), nil).ResolveResource(res, ResolvedDependencies{})
	if err != nil {
		t.Fatalf("ResolveResource returned error: %v", err)
	}
	if got.Env["A"] != "C+B+A" {
		t.Errorf("Env[A] = %q, want C+B+A", got.Env["A"])
	}
}

func TestResolveResource_EnvTemplateHostPort(t *testing.T) {
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "cache", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Port: 6379,
			Env: []manifest.EnvVar{
				{Name: "SELF_CONN", Template: "redis://{{.host}}:{{.port}}"},
			},
		},
	}

	got, err := NewLiveResolver(newFakeSecrets(), nil).ResolveResource(res, ResolvedDependencies{})
	if err != nil {
		t.Fatalf("ResolveResource returned error: %v", err)
	}
	if got.Env["SELF_CONN"] != "redis://team-a.cache:6379" {
		t.Errorf("Env[SELF_CONN] = %q, want redis://team-a.cache:6379", got.Env["SELF_CONN"])
	}
}

func TestResolveResource_EnvTemplateCycle(t *testing.T) {
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "cycle", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Env: []manifest.EnvVar{
				{Name: "A", Template: "{{.B}}"},
				{Name: "B", Template: "{{.A}}"},
			},
		},
	}

	_, err := NewLiveResolver(newFakeSecrets(), nil).ResolveResource(res, ResolvedDependencies{})
	if err == nil || !strings.Contains(err.Error(), "template cycle") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

func TestResolveResource_EnvValueFromResource(t *testing.T) {
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "cache", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Env:     []manifest.EnvVar{{Name: "UPSTREAM_DB_HOST", ValueFrom: "resource.pg.host"}},
			Outputs: []manifest.Output{{Name: "UPSTREAM_DB_HOST"}},
		},
	}
	deps := ResolvedDependencies{Resources: map[string]map[string]string{"pg": {"host": "team-a.pg"}}}

	got, err := NewLiveResolver(newFakeSecrets(), nil).ResolveResource(res, deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Env["UPSTREAM_DB_HOST"] != "team-a.pg" {
		t.Errorf("Env[UPSTREAM_DB_HOST] = %q, want team-a.pg", got.Env["UPSTREAM_DB_HOST"])
	}
}

func TestResolveResource_OutputBareUnknownFails(t *testing.T) {
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "db", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Outputs: []manifest.Output{{Name: "mystery"}},
		},
	}
	_, err := NewLiveResolver(newFakeSecrets(), nil).ResolveResource(res, ResolvedDependencies{})
	if err == nil || !strings.Contains(err.Error(), "matches no env var or built-in") {
		t.Errorf("expected no-match error, got: %v", err)
	}
}

func TestResolveResource_PortExportRequiresSpecPort(t *testing.T) {
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "db", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Outputs: []manifest.Output{{Name: "port"}},
		},
	}
	_, err := NewLiveResolver(newFakeSecrets(), nil).ResolveResource(res, ResolvedDependencies{})
	if err == nil || !strings.Contains(err.Error(), "requires spec.port") {
		t.Errorf("expected port-requires-spec.port error, got: %v", err)
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

	env, err := NewLiveResolver(nil, nil).ResolveApplication(app, ResolvedDependencies{Resources: resolved})
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

func TestResolveApplication_VaultEnv(t *testing.T) {
	vault := newFakeVault("myproject/prod/API_KEY", "key-xyz")
	app := &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Env: []manifest.EnvVar{
				{Name: "STATIC", Value: "hello"},
				{Name: "API_KEY", ValueFrom: "vault:myproject/prod/API_KEY"},
			},
		},
	}

	env, err := NewLiveResolver(nil, vault).ResolveApplication(app, ResolvedDependencies{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env["API_KEY"] != "key-xyz" {
		t.Errorf("API_KEY = %q, want key-xyz", env["API_KEY"])
	}
	if env["STATIC"] != "hello" {
		t.Errorf("STATIC = %q, want hello", env["STATIC"])
	}
}

func TestResolveApplication_VaultEnv_NoPlugin(t *testing.T) {
	app := &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Env: []manifest.EnvVar{
				{Name: "API_KEY", ValueFrom: "vault:proj/prod/KEY"},
			},
		},
	}

	_, err := NewLiveResolver(nil, nil).ResolveApplication(app, ResolvedDependencies{})
	if err == nil || !strings.Contains(err.Error(), "no secrets plugin") {
		t.Errorf("expected no-plugin error, got: %v", err)
	}
}

func TestResolveApplication_VaultEnv_ErrorContainsPath(t *testing.T) {
	vault := newFakeVault() // empty vault — will return error
	app := &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Env: []manifest.EnvVar{
				{Name: "API_KEY", ValueFrom: "vault:proj/prod/MISSING"},
			},
		},
	}

	_, err := NewLiveResolver(nil, vault).ResolveApplication(app, ResolvedDependencies{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "proj/prod/MISSING") {
		t.Errorf("error should contain secret path, got: %v", err)
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
	_, err := NewLiveResolver(nil, nil).ResolveApplication(app, ResolvedDependencies{Resources: map[string]map[string]string{}})
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
		env, err := NewLiveResolver(nil, nil).ResolveApplication(app, ResolvedDependencies{})
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
		env, err := NewLiveResolver(nil, nil).ResolveApplication(app, ResolvedDependencies{Resources: resolved})
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
		env, err := NewLiveResolver(nil, nil).ResolveApplication(app, ResolvedDependencies{})
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
		_, err := NewLiveResolver(nil, nil).ResolveApplication(app, ResolvedDependencies{})
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
		env, err := NewLiveResolver(nil, nil).ResolveApplication(app, ResolvedDependencies{})
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
		env, err := NewLiveResolver(nil, nil).ResolveApplication(app, ResolvedDependencies{Applications: resolvedApps})
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

	env, err := NewLiveResolver(nil, nil).ResolveApplication(app, deps)
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
		_, err := NewLiveResolver(nil, nil).ResolveApplication(badApp, deps)
		if err == nil || !strings.Contains(err.Error(), "has no resolved output \"url\"") {
			t.Errorf("expected missing output error, got: %v", err)
		}
	})
}

// --- DryRunResolver vault placeholder tests (T018) ---

func TestDryRunResolver_VaultPlaceholder_AppEnv(t *testing.T) {
	app := &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Env: []manifest.EnvVar{
				{Name: "STATIC", Value: "hello"},
				{Name: "API_KEY", ValueFrom: "vault:proj/prod/API_KEY"},
			},
		},
	}

	r := NewDryRunResolver()
	env, err := r.ResolveApplication(app, ResolvedDependencies{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env["STATIC"] != "hello" {
		t.Errorf("STATIC = %q, want hello", env["STATIC"])
	}
	if env["API_KEY"] != "[VAULT:proj/prod/API_KEY]" {
		t.Errorf("API_KEY = %q, want [VAULT:proj/prod/API_KEY]", env["API_KEY"])
	}
}

func TestDryRunResolver_ResourceEnvAndExports(t *testing.T) {
	res := &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "db", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Port: 5432,
			Env: []manifest.EnvVar{
				{Name: "PW", Generated: true},
				{Name: "SECRET", ValueFrom: "vault:a/b/c"},
				{Name: "DB", Value: "app"},
			},
			Outputs: []manifest.Output{
				{Name: "DB"},
				{Name: "host"},
				{Name: "port"},
				{Name: "URL", Template: "{{.DB}}:{{.PW}}"},
			},
		},
	}

	r := NewDryRunResolver()
	got, err := r.ResolveResource(res, ResolvedDependencies{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Env["PW"] != "[GENERATED]" {
		t.Errorf("Env[PW] = %q, want [GENERATED]", got.Env["PW"])
	}
	if got.Env["SECRET"] != "[VAULT:a/b/c]" {
		t.Errorf("Env[SECRET] = %q, want [VAULT:a/b/c]", got.Env["SECRET"])
	}
	if got.Exports["port"] != "[PORT]" {
		t.Errorf("Exports[port] = %q, want [PORT]", got.Exports["port"])
	}
	if got.Exports["URL"] != "app:[GENERATED]" {
		t.Errorf("Exports[URL] = %q, want app:[GENERATED]", got.Exports["URL"])
	}
	if _, ok := got.Exports["PW"]; ok {
		t.Error("PW is not listed in output and must not be exported")
	}
}

func TestDryRunResolver_VaultPlaceholder_NoNetworkCall(t *testing.T) {
	// This test simply verifies dry-run does not panic or error
	// even when there is no live vault configured.
	app := &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Env: []manifest.EnvVar{
				{Name: "SECRET", ValueFrom: "vault:a/b/c"},
			},
		},
	}
	r := NewDryRunResolver()
	env, err := r.ResolveApplication(app, ResolvedDependencies{})
	if err != nil {
		t.Fatalf("dry-run must not error without vault: %v", err)
	}
	if env["SECRET"] != "[VAULT:a/b/c]" {
		t.Errorf("SECRET = %q, want [VAULT:a/b/c]", env["SECRET"])
	}
}
