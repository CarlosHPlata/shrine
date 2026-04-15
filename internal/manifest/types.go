package manifest

// Metadata holds fields shared by all manifest kinds.
type Metadata struct {
	Name   string   `yaml:"name"`
	Owner  string   `yaml:"owner"`
	Access []string `yaml:"access,omitempty"`
}

// Used in Application spec
type Dependency struct {
	Kind  string `yaml:"kind"`
	Name  string `yaml:"name"`
	Owner string `yaml:"owner"`
}

// Used in Application spec
type EnvVar struct {
	Name      string `yaml:"name"`
	Value     string `yaml:"value,omitempty"`
	ValueFrom string `yaml:"valueFrom,omitempty"`
}

// Used in Application spec
type Routing struct {
	Domain     string `yaml:"domain"`
	PathPrefix string `yaml:"pathPrefix,omitempty"`
}

// Used in Application spec
type Networking struct {
	ExposeToPlatform bool `yaml:"exposeToPlatform,omitempty"`
}

type ApplicationSpec struct {
	Image        string       `yaml:"image"`
	Port         int          `yaml:"port,omitempty"`
	Replicas     int          `yaml:"replicas,omitempty"`
	Routing      Routing      `yaml:"routing,omitempty"`
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
	Env          []EnvVar     `yaml:"env,omitempty"`
	Networking   Networking   `yaml:"networking,omitempty"`
}

type ResourceSpec struct {
	Type       string     `yaml:"type"`
	Version    string     `yaml:"version"`
	Networking Networking `yaml:"networking,omitempty"`
}

type Quotas struct {
	MaxApps              int      `yaml:"maxApps,omitempty"`
	MaxResources         int      `yaml:"maxResources,omitempty"`
	AllowedResourceTypes []string `yaml:"allowedResourceTypes,omitempty"`
}

type TeamSpec struct {
	DisplayName  string `yaml:"displayName"`
	Contact      string `yaml:"contact"`
	Quotas       Quotas `yaml:"quotas"`
	RegistryUser string `yaml:"registryUser"`
}

type TypeMeta struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
}

type ApplicationManifest struct {
	TypeMeta `yaml:",inline"`
	Metadata Metadata        `yaml:"metadata"`
	Spec     ApplicationSpec `yaml:"spec"`
}

type ResourceManifest struct {
	TypeMeta `yaml:",inline"`
	Metadata Metadata     `yaml:"metadata"`
	Spec     ResourceSpec `yaml:"spec"`
}

type TeamManifest struct {
	TypeMeta `yaml:",inline"`
	Metadata Metadata `yaml:"metadata"`
	Spec     TeamSpec `yaml:"spec"`
}
