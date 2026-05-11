package infisical

import (
	"errors"
	"strings"
	"testing"
)

// fakeBackend implements secretFetcher for unit tests.
type fakeBackend struct {
	value       string
	err         error
	projectMap  map[string]string // slug or name → UUID
	resolveErr  error
	resolveHits int
	lastProj    string
	lastEnv     string
	lastKey     string
}

func (f *fakeBackend) retrieve(projectUUID, env, key string) (string, error) {
	f.lastProj, f.lastEnv, f.lastKey = projectUUID, env, key
	if f.err != nil {
		return "", f.err
	}
	return f.value, nil
}

func (f *fakeBackend) resolveProject(input string) (string, error) {
	f.resolveHits++
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	if uuid, ok := f.projectMap[input]; ok {
		return uuid, nil
	}
	return "", errors.New("not found: " + input)
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

func TestGetSecret_WithUUID_BypassesResolve(t *testing.T) {
	fb := &fakeBackend{value: "supersecret"}
	p := &InfisicalPlugin{fetcher: fb}

	got, err := p.GetSecret("486975e5-b3a2-49d4-8c54-9702270c25ce/prod/DB_PASSWORD")
	if err != nil {
		t.Fatalf("GetSecret returned unexpected error: %v", err)
	}
	if got != "supersecret" {
		t.Fatalf("expected %q, got %q", "supersecret", got)
	}
	if fb.resolveHits != 0 {
		t.Errorf("expected no resolveProject calls for UUID, got %d", fb.resolveHits)
	}
	if fb.lastProj != "486975e5-b3a2-49d4-8c54-9702270c25ce" {
		t.Errorf("UUID should be passed through unchanged, got %q", fb.lastProj)
	}
}

func TestGetSecret_WithSlug_ResolvesToUUID(t *testing.T) {
	fb := &fakeBackend{
		value:      "supersecret",
		projectMap: map[string]string{"my-app": "486975e5-b3a2-49d4-8c54-9702270c25ce"},
	}
	p := &InfisicalPlugin{fetcher: fb}

	got, err := p.GetSecret("my-app/prod/DB_PASSWORD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "supersecret" {
		t.Fatalf("expected %q, got %q", "supersecret", got)
	}
	if fb.resolveHits != 1 {
		t.Errorf("expected 1 resolveProject call, got %d", fb.resolveHits)
	}
	if fb.lastProj != "486975e5-b3a2-49d4-8c54-9702270c25ce" {
		t.Errorf("expected resolved UUID, got %q", fb.lastProj)
	}
}

func TestGetSecret_PassesCorrectFieldsToBackend(t *testing.T) {
	fb := &fakeBackend{
		value:      "val",
		projectMap: map[string]string{"myproj": "486975e5-b3a2-49d4-8c54-9702270c25ce"},
	}
	p := &InfisicalPlugin{fetcher: fb}

	_, err := p.GetSecret("myproj/staging/API_KEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fb.lastEnv != "staging" {
		t.Errorf("expected env %q, got %q", "staging", fb.lastEnv)
	}
	if fb.lastKey != "API_KEY" {
		t.Errorf("expected key %q, got %q", "API_KEY", fb.lastKey)
	}
}

func TestGetSecret_UnknownProject_ErrorContainsPath(t *testing.T) {
	fb := &fakeBackend{projectMap: map[string]string{}}
	p := &InfisicalPlugin{fetcher: fb}

	_, err := p.GetSecret("does-not-exist/prod/KEY")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "vault:does-not-exist/prod/KEY") {
		t.Errorf("error must contain path, got: %v", err)
	}
}

func TestGetSecret_RetrieveError_ContainsPath_NotValue(t *testing.T) {
	fb := &fakeBackend{
		err:        errors.New("not found"),
		projectMap: map[string]string{"myproj": "486975e5-b3a2-49d4-8c54-9702270c25ce"},
	}
	p := &InfisicalPlugin{fetcher: fb}

	_, err := p.GetSecret("myproj/prod/DB_PASSWORD")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "vault:myproj/prod/DB_PASSWORD") {
		t.Fatalf("error must contain path, got: %v", err)
	}
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

func TestIsUUID(t *testing.T) {
	cases := map[string]bool{
		"486975e5-b3a2-49d4-8c54-9702270c25ce": true,
		"486975E5-B3A2-49D4-8C54-9702270C25CE": true,
		"my-project":                           false,
		"my-app-1":                             false,
		"":                                     false,
		"not-a-uuid-at-all":                    false,
		"486975e5-b3a2-49d4-8c54":              false, // too short
	}
	for input, want := range cases {
		if got := isUUID(input); got != want {
			t.Errorf("isUUID(%q) = %v, want %v", input, got, want)
		}
	}
}
