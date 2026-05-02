package traefik

type staticConfig struct {
	EntryPoints map[string]entryPoint `yaml:"entryPoints"`
	API         *apiConfig            `yaml:"api,omitempty"`
	Providers   providersConfig       `yaml:"providers"`
}

type entryPoint struct {
	Address string `yaml:"address"`
}

type apiConfig struct {
	Dashboard bool `yaml:"dashboard"`
}

type providersConfig struct {
	File fileProvider `yaml:"file"`
}

type fileProvider struct {
	Directory string `yaml:"directory"`
	Watch     bool   `yaml:"watch"`
}

type httpConfig struct {
	Middlewares map[string]middleware `yaml:"middlewares,omitempty"`
	Routers     map[string]router     `yaml:"routers,omitempty"`
	Services    map[string]service    `yaml:"services,omitempty"`
}

type middleware struct {
	BasicAuth   *basicAuth   `yaml:"basicAuth,omitempty"`
	StripPrefix *stripPrefix `yaml:"stripPrefix,omitempty"`
}

type basicAuth struct {
	Users []string `yaml:"users"`
}

type stripPrefix struct {
	Prefixes []string `yaml:"prefixes"`
}

type router struct {
	Rule        string    `yaml:"rule"`
	Service     string    `yaml:"service"`
	EntryPoints []string  `yaml:"entryPoints"`
	Middlewares []string  `yaml:"middlewares,omitempty"`
	TLS         *tlsBlock `yaml:"tls,omitempty"`
}

type tlsBlock struct{}

type service struct {
	LoadBalancer loadBalancer `yaml:"loadBalancer"`
}

type loadBalancer struct {
	Servers []server `yaml:"servers"`
}

type server struct {
	URL string `yaml:"url"`
}
