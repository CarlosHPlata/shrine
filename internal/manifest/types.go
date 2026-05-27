package manifest

import "strings"

const (
	ApplicationKind = "Application"
	ResourceKind    = "Resource"
	TeamKind        = "Team"
)

const (
	ImagePullPolicyAlways       = "Always"
	ImagePullPolicyIfNotPresent = "IfNotPresent"
)

// Metadata holds fields shared by all manifest kinds.
type Metadata struct {
	ResourceID string   `yaml:"resourceId,omitempty" json:"resourceId,omitempty"`
	Name       string   `yaml:"name"`
	Owner      string   `yaml:"owner"`
	Access     []string `yaml:"access,omitempty"`
}

// Used in Application spec
type Dependency struct {
	Kind  string `yaml:"kind"`
	Name  string `yaml:"name"`
	Owner string `yaml:"owner"`
}

// Used in Application and Resource specs. Generated is valid only on Resource
// env (auto-minted secret); Application env must not set it.
type EnvVar struct {
	Name      string `yaml:"name"`
	Value     string `yaml:"value,omitempty"`
	ValueFrom string `yaml:"valueFrom,omitempty"`
	Template  string `yaml:"template,omitempty" json:"template,omitempty"`
	Generated bool   `yaml:"generated,omitempty" json:"generated,omitempty"`
}

type RoutingAlias struct {
	Host        string `yaml:"host"`
	PathPrefix  string `yaml:"pathPrefix,omitempty"`
	StripPrefix *bool  `yaml:"stripPrefix,omitempty"`
	TLS         bool   `yaml:"tls,omitempty"`
}

// Used in Application spec
type Routing struct {
	Domain     string         `yaml:"domain"`
	PathPrefix string         `yaml:"pathPrefix,omitempty"`
	Aliases    []RoutingAlias `yaml:"aliases,omitempty"`
}

// Used in App and Res spec
type Networking struct {
	ExposeToPlatform bool `yaml:"exposeToPlatform,omitempty"`
}

type VolumeMount struct {
	Name      string `yaml:"name"`
	MountPath string `yaml:"mountPath"`
}

type ApplicationSpec struct {
	Image           string        `yaml:"image"`
	Port            int           `yaml:"port,omitempty"`
	Replicas        int           `yaml:"replicas,omitempty"`
	Routing         Routing       `yaml:"routing,omitempty"`
	Dependencies    []Dependency  `yaml:"dependencies,omitempty"`
	Env             []EnvVar      `yaml:"env,omitempty"`
	Networking      Networking    `yaml:"networking,omitempty"`
	Volumes         []VolumeMount `yaml:"volumes,omitempty"`
	ImagePullPolicy string        `yaml:"imagePullPolicy,omitempty"`
}

// Output declares one item in a Resource's export allowlist. Its only valid
// fields are Name and an optional Template. Value, Generated, and ValueFrom are
// retained solely so pre-split manifests still unmarshal and can be rejected
// with an actionable migration error (see validateResourceSpec).
type Output struct {
	Name      string `yaml:"name" json:"name"`
	Template  string `yaml:"template,omitempty" json:"template,omitempty"`
	Value     string `yaml:"value,omitempty" json:"value,omitempty"`         // deprecated — rejected if set
	Generated bool   `yaml:"generated,omitempty" json:"generated,omitempty"` // deprecated — rejected if set
	ValueFrom string `yaml:"valueFrom,omitempty" json:"valueFrom,omitempty"` // deprecated — rejected if set
}

type ResourceSpec struct {
	Type            string        `yaml:"type"`
	Version         string        `yaml:"version"`
	Port            int           `yaml:"port,omitempty"`
	Image           string        `yaml:"image,omitempty"`
	Dependencies    []Dependency  `yaml:"dependencies,omitempty"`
	Env             []EnvVar      `yaml:"env,omitempty"`
	Outputs         []Output      `yaml:"outputs,omitempty"`
	Networking      Networking    `yaml:"networking,omitempty"`
	Volumes         []VolumeMount `yaml:"volumes,omitempty"`
	ImagePullPolicy string        `yaml:"imagePullPolicy,omitempty"`
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

func EffectivePullPolicy(image string, declared string) string {
	if declared != "" {
		return declared
	}
	colonIdx := strings.LastIndex(image, ":")
	if colonIdx == -1 {
		return ImagePullPolicyAlways
	}
	tag := image[colonIdx+1:]
	if tag == "" || tag == "latest" {
		return ImagePullPolicyAlways
	}
	return ImagePullPolicyIfNotPresent
}
