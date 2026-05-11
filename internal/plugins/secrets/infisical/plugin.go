package infisical

import (
	"context"
	"fmt"
	"strings"

	infisicalsdk "github.com/infisical/go-sdk"

	"github.com/CarlosHPlata/shrine/internal/config"
)

// secretFetcher is a narrow internal interface that wraps the one SDK call we
// need. It exists solely to make the plugin testable without mocking the entire
// Infisical SDK.
type secretFetcher interface {
	retrieve(project, env, key string) (string, error)
}

// sdkFetcher adapts the real Infisical SDK client to secretFetcher.
type sdkFetcher struct {
	client infisicalsdk.InfisicalClientInterface
}

func (s *sdkFetcher) retrieve(project, env, key string) (string, error) {
	secret, err := s.client.Secrets().Retrieve(infisicalsdk.RetrieveSecretOptions{
		ProjectID:   project,
		Environment: env,
		SecretKey:   key,
		SecretPath:  "/",
	})
	if err != nil {
		return "", err
	}
	return secret.SecretValue, nil
}

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

	return &InfisicalPlugin{fetcher: &sdkFetcher{client: client}}, nil
}

// IsActive returns true when the plugin was constructed with a live fetcher.
func (p *InfisicalPlugin) IsActive() bool {
	return p != nil && p.fetcher != nil
}

// GetSecret fetches the secret at path from Infisical.
// path must have exactly 3 slash-separated components: project/environment/secret-name.
// Errors include the path but never the secret value.
func (p *InfisicalPlugin) GetSecret(path string) (string, error) {
	project, env, key, err := parsePath(path)
	if err != nil {
		return "", err
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
