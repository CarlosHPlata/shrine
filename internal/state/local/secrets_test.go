package local

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/state"
)

func TestSecretStore_LoadTeam(t *testing.T) {
	tmpDir := t.TempDir()
	team := "team-a"
	teamDir := filepath.Join(tmpDir, team)
	if err := os.MkdirAll(teamDir, 0700); err != nil {
		t.Fatalf("failed to create team dir: %v", err)
	}

	data := `
# Team secrets
DB_PASSWORD=secret-pass
  # indented comment
API_KEY=12345 # inline comment

EMPTY_VAL=
`
	if err := os.WriteFile(filepath.Join(teamDir, "secrets.env"), []byte(data), 0600); err != nil {
		t.Fatalf("failed to setup test file: %v", err)
	}

	store, err := NewSecretStore(tmpDir)
	if err != nil {
		t.Fatalf("NewSecretStore failed: %v", err)
	}

	s := store.(*SecretStore)
	secrets, err := s.loadTeam(team)
	if err != nil {
		t.Fatalf("loadTeam failed: %v", err)
	}

	expected := map[string]string{
		"DB_PASSWORD": "secret-pass",
		"API_KEY":     "12345",
		"EMPTY_VAL":   "",
	}

	if len(secrets) != len(expected) {
		t.Errorf("got %d secrets, want %d", len(secrets), len(expected))
	}

	for k, v := range expected {
		if got := secrets[k]; got != v {
			t.Errorf("key %s: got %q, want %q", k, got, v)
		}
	}
}

func TestSecretStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	team := "team-x"

	// 1. Generate a secret
	store1, _ := NewSecretStore(tmpDir)
	val1, generated, err := store1.GetOrGenerate(team, "SECRET_KEY", 16)
	if err != nil || !generated {
		t.Fatalf("first GetOrGenerate failed: err=%v, generated=%v", err, generated)
	}

	// 2. Re-load in a new store instance
	store2, _ := NewSecretStore(tmpDir)
	val2, err := store2.Get(team, "SECRET_KEY")
	if err != nil {
		t.Fatalf("Get on re-loaded store failed: %v", err)
	}

	if val1 != val2 {
		t.Errorf("persistence failed: got %q, want %q", val2, val1)
	}
}

func TestSecretStore_Interface(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewSecretStore(tmpDir)
	team := "team-a"

	// 1. GetOrGenerate (New)
	val1, gen1, err := store.GetOrGenerate(team, "S1", 10)
	if err != nil || !gen1 {
		t.Errorf("GetOrGenerate (new) failed: %v, %v", err, gen1)
	}

	// 2. GetOrGenerate (Existing - Idempotency)
	val2, gen2, err := store.GetOrGenerate(team, "S1", 10)
	if err != nil || gen2 {
		t.Errorf("GetOrGenerate (existing) failed: %v, %v", err, gen2)
	}
	if val1 != val2 {
		t.Errorf("idempotency failed: %q != %q", val1, val2)
	}

	// 3. Get
	got, err := store.Get(team, "S1")
	if err != nil || got != val1 {
		t.Errorf("Get failed: %v, got %q", err, got)
	}

	_, err = store.Get(team, "NON_EXISTENT")
	if !errors.Is(err, state.ErrSecretNotFound) {
		t.Errorf("Get non-existent: got %v, want %v", err, state.ErrSecretNotFound)
	}

	// 4. List
	all, err := store.List(team)
	if err != nil || len(all) != 1 || all["S1"] != val1 {
		t.Errorf("List failed: %v, %v", err, all)
	}
}

func TestSecretStore_DefensiveCopy(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewSecretStore(tmpDir)
	team := "team-a"

	store.GetOrGenerate(team, "K1", 10)

	all, _ := store.List(team)
	all["K1"] = "MODIFIED"

	got, _ := store.Get(team, "K1")
	if got == "MODIFIED" {
		t.Error("List returned a reference to internal state; defensive copy failed")
	}
}
