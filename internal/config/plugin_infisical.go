package config

// InfisicalPluginConfig holds connection parameters for a self-hosted Infisical
// instance. Authentication uses Machine Identity Universal Auth.
type InfisicalPluginConfig struct {
	URL          string `yaml:"url"`
	ClientID     string `yaml:"client-id"`
	ClientSecret string `yaml:"client-secret"`
}
