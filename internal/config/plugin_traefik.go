package config

type TraefikPluginConfig struct {
	Image      string                  `yaml:"image,omitempty"`
	RoutingDir string                  `yaml:"routing-dir,omitempty"`
	Port       int                     `yaml:"port,omitempty"`
	Dashboard  *TraefikDashboardConfig `yaml:"dashboard,omitempty"`
}

type TraefikDashboardConfig struct {
	Port     int    `yaml:"port"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

func (p *TraefikPluginConfig) ResolveRoutingDir(specsDir string) (string, error) {
	return resolvePath(
		[]string{p.RoutingDir, specsDir},
		"no routing directory: set --path/-p flag or routing-dir in config.yml",
	)
}
