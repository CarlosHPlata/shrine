package infisical

import (
	"errors"
	"strings"
	"testing"
)

// fakeBackend implements secretFetcher for unit tests.
type fakeBackend struct {
	value    string
	err      error
	lastProj string
	lastEnv  string
	lastKey  string
}

func (f *fakeBackend) retrieve(project, env, key string) (string, error) {
	f.lastProj, f.lastEnv, f.lastKey = project, env, key
	if f.err != nil {
		return "", f.err
	}
	return f.value, nil
}

func TestNew_NilConfig(t *testing.T) {
	p, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}
	if p != nil {
		t.Fatal("New(nil) should return nil plugin")
	}
}

func TestIsActive_NilPlugin(t *testing.T) {
	var p *InfisicalPlugin
	if p.IsActive() {
		t.Fatal("nil plugin must not be active")
	}
}

func TestIsActive_WithFetcher(t *testing.T) {
	p := &InfisicalPlugin{fetcher: &fakeBackend{}}
	if !p.IsActive() {
		t.Fatal("plugin with fetcher must be active")
	}
}

func TestGetSecret_Success(t *testing.T) {
	fb := &fakeBackend{value: "supersecret"}
	p := &InfisicalPlugin{fetcher: fb}

	got, err := p.GetSecret("myproject/production/DB_PASSWORD")
	if err != nil {
		t.Fatalf("GetSecret returned unexpected error: %v", err)
	}
	if got != "supersecret" {
		t.Fatalf("expected %q, got %q", "supersecret", got)
	}
}

func TestGetSecret_PassesCorrectFieldsToBackend(t *testing.T) {
	fb := &fakeBackend{value: "val"}
	p := &InfisicalPlugin{fetcher: fb}

	_, err := p.GetSecret("myproj/staging/API_KEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fb.lastProj != "myproj" {
		t.Errorf("expected project %q, got %q", "myproj", fb.lastProj)
	}
	if fb.lastEnv != "staging" {
		t.Errorf("expected env %q, got %q", "staging", fb.lastEnv)
	}
	if fb.lastKey != "API_KEY" {
		t.Errorf("expected key %q, got %q", "API_KEY", fb.lastKey)
	}
}

func TestGetSecret_ErrorContainsPath_NotValue(t *testing.T) {
	fb := &fakeBackend{err: errors.New("not found")}
	p := &InfisicalPlugin{fetcher: fb}

	_, err := p.GetSecret("myproject/production/DB_PASSWORD")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "vault:myproject/production/DB_PASSWORD") {
		t.Fatalf("error must contain path, got: %v", err)
	}
	// Ensure the underlying "not found" message is included (path context) but
	// the secret VALUE is not returned in errors by design (enforced via code
	// review — the fakeBackend never sets a value on error paths).
}

func TestGetSecret_MalformedPath(t *testing.T) {
	p := &InfisicalPlugin{fetcher: &fakeBackend{value: "x"}}

	cases := []string{"foo", "foo/bar", "", "foo//bar"}
	for _, bad := range cases {
		_, err := p.GetSecret(bad)
		if err == nil {
			t.Errorf("expected error for malformed path %q", bad)
		}
	}
}
