package infisical

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	infisicalsdk "github.com/infisical/go-sdk"

	"github.com/CarlosHPlata/shrine/internal/config"
)

// secretFetcher is a narrow internal interface that wraps the SDK calls we need.
// It exists solely to make the plugin testable without mocking the entire SDK.
type secretFetcher interface {
	// retrieve fetches the secret value identified by projectUUID/env/key.
	retrieve(projectUUID, env, key string) (string, error)
	// resolveProject maps a project slug or display name to its UUID.
	// Implementations should cache the result for the plugin lifetime.
	resolveProject(input string) (string, error)
}

// sdkFetcher adapts the real Infisical SDK + a direct HTTP call for project listing
// (the SDK does not expose a workspace lookup, so we hit /api/v1/workspace ourselves).
type sdkFetcher struct {
	client infisicalsdk.InfisicalClientInterface
	url    string

	cacheOnce sync.Once
	cache     map[string]string
	cacheErr  error
}

func (s *sdkFetcher) retrieve(projectUUID, env, key string) (string, error) {
	secret, err := s.client.Secrets().Retrieve(infisicalsdk.RetrieveSecretOptions{
		ProjectID:   projectUUID,
		Environment: env,
		SecretKey:   key,
		SecretPath:  "/",
	})
	if err != nil {
		return "", err
	}
	return secret.SecretValue, nil
}

func (s *sdkFetcher) resolveProject(input string) (string, error) {
	s.cacheOnce.Do(func() { s.cache, s.cacheErr = s.fetchWorkspaces() })
	if s.cacheErr != nil {
		return "", s.cacheErr
	}
	if uuid, ok := s.cache[input]; ok {
		return uuid, nil
	}
	visible := make([]string, 0, len(s.cache))
	seen := make(map[string]bool, len(s.cache))
	for k := range s.cache {
		if !seen[k] {
			visible = append(visible, k)
			seen[k] = true
		}
	}
	return "", fmt.Errorf("project %q not found in vault (visible: %s)", input, strings.Join(visible, ", "))
}

// fetchWorkspaces queries Infisical for the projects this identity can see and
// builds a single lookup map keyed by both slug and display name.
func (s *sdkFetcher) fetchWorkspaces() (map[string]string, error) {
	token := s.client.Auth().GetAccessToken()
	req, err := http.NewRequest(http.MethodGet, s.url+"/api/v1/workspace", nil)
	if err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listing workspaces: HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Workspaces []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Slug string `json:"slug"`
		} `json:"workspaces"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding workspace list: %w", err)
	}

	out := make(map[string]string, len(payload.Workspaces)*2)
	for _, w := range payload.Workspaces {
		if w.Slug != "" {
			out[w.Slug] = w.ID
		}
		if w.Name != "" {
			out[w.Name] = w.ID
		}
	}
	return out, nil
}

// uuidPattern matches a canonical UUID v4 string.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool { return uuidPattern.MatchString(s) }

// InfisicalPlugin implements secrets.SecretsPlugin against a self-hosted
// Infisical instance using Machine Identity Universal Auth.
type InfisicalPlugin struct {
	fetcher secretFetcher
}

// New constructs an InfisicalPlugin and authenticates to Infisical.
// Returns nil, nil when cfg is nil (plugin inactive).
func New(cfg *config.InfisicalPluginConfig) (*InfisicalPlugin, error) {
	if cfg == nil {
		return nil, nil
	}

	client := infisicalsdk.NewInfisicalClient(context.Background(), infisicalsdk.Config{
		SiteUrl:          cfg.URL,
		AutoTokenRefresh: true,
	})

	if _, err := client.Auth().UniversalAuthLogin(cfg.ClientID, cfg.ClientSecret); err != nil {
		return nil, fmt.Errorf("infisical: authentication failed: %w", err)
	}

	return &InfisicalPlugin{
		fetcher: &sdkFetcher{client: client, url: cfg.URL},
	}, nil
}

// IsActive returns true when the plugin was constructed with a live fetcher.
func (p *InfisicalPlugin) IsActive() bool {
	return p != nil && p.fetcher != nil
}

// GetSecret fetches the secret at path from Infisical.
// path must have exactly 3 slash-separated components: project/environment/secret-name.
// The project component may be a UUID (used directly) or a slug/display name
// (resolved to a UUID on first use and cached).
// Errors include the path but never the secret value.
func (p *InfisicalPlugin) GetSecret(path string) (string, error) {
	project, env, key, err := parsePath(path)
	if err != nil {
		return "", err
	}

	if !isUUID(project) {
		resolved, err := p.fetcher.resolveProject(project)
		if err != nil {
			return "", fmt.Errorf("vault:%s: %w", path, err)
		}
		project = resolved
	}

	val, err := p.fetcher.retrieve(project, env, key)
	if err != nil {
		return "", fmt.Errorf("vault:%s: %w", path, err)
	}

	return val, nil
}

func parsePath(path string) (project, env, key string, err error) {
	parts := strings.SplitN(path, "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("vault:%s: path must have exactly 3 non-empty components (project/environment/secret-name)", path)
	}
	return parts[0], parts[1], parts[2], nil
}
