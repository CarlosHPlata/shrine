package local

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/CarlosHPlata/shrine/internal/state"
)

type SecretStore struct {
	mu      sync.Mutex
	baseDir string
}

func NewSecretStore(baseDir string) (state.SecretStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("creating state directory: %w", err)
	}

	s := &SecretStore{
		baseDir: baseDir,
	}

	return s, nil
}

func (s *SecretStore) GetOrGenerate(team string, key string, length int) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	secrets, err := s.loadTeam(team)
	if err != nil {
		return "", false, err
	}

	if val, ok := secrets[key]; ok {
		return val, false, nil
	}

	// Generate new secret
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", false, fmt.Errorf("generating secret: %w", err)
	}
	val := base64.RawURLEncoding.EncodeToString(b)

	secrets[key] = val
	if err := s.saveTeam(team, secrets); err != nil {
		return "", false, err
	}

	return val, true, nil
}

func (s *SecretStore) Get(team string, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	secrets, err := s.loadTeam(team)
	if err != nil {
		return "", err
	}

	val, ok := secrets[key]
	if !ok {
		return "", state.ErrSecretNotFound
	}

	return val, nil
}

func (s *SecretStore) List(team string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	secrets, err := s.loadTeam(team)
	if err != nil {
		return nil, err
	}

	res := make(map[string]string, len(secrets))
	maps.Copy(res, secrets)

	return res, nil
}

func (s *SecretStore) loadTeam(team string) (map[string]string, error) {
	path := filepath.Join(s.baseDir, team, "secrets.env")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("opening secrets for team %q: %w", team, err)
	}
	defer f.Close()

	secrets := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// Strip comments and leading/trailing whitespace
		line, _, _ := strings.Cut(scanner.Text(), "#")
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			secrets[parts[0]] = parts[1]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading secrets for team %q: %w", team, err)
	}

	return secrets, nil
}

func (s *SecretStore) saveTeam(team string, secrets map[string]string) error {
	teamDir := filepath.Join(s.baseDir, team)
	if err := os.MkdirAll(teamDir, 0700); err != nil {
		return fmt.Errorf("creating team directory for %q: %w", team, err)
	}

	// CreateTemp uses 0600 by default on Unix
	tmp, err := os.CreateTemp(teamDir, "secrets-*.env.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary secrets file: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	// Sort keys for deterministic output
	keys := make([]string, 0, len(secrets))
	for k := range secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if _, err := fmt.Fprintf(tmp, "%s=%s\n", k, secrets[k]); err != nil {
			return fmt.Errorf("writing to temporary secrets file: %w", err)
		}
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temporary secrets file: %w", err)
	}

	destPath := filepath.Join(teamDir, "secrets.env")
	if err := os.Rename(tmp.Name(), destPath); err != nil {
		return fmt.Errorf("finalizing secrets file for %q: %w", team, err)
	}

	return nil
}
