package manifest

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

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

// The host-port block reserved for automatic publish allocation. Explicit
// hostPort values are excluded from it at validate time so an explicit claim
// can never race an automatic allocation made in the same deploy.
const (
	FirstAutoHostPort = 30000
	LastAutoHostPort  = 32767
)

// Publish declares loopback host publishing of the workload's service port.
// HostPort 0 means "allocate automatically from the reserved range".
type Publish struct {
	HostPort int `yaml:"hostPort,omitempty"`
}

// Used in App and Res spec
type Networking struct {
	ExposeToPlatform bool     `yaml:"exposeToPlatform,omitempty"`
	Publish          *Publish `yaml:"publish,omitempty"`
}

// UnmarshalYAML accepts both publish forms — `publish: true|false` and
// `publish: {hostPort: N}` — normalizing false/null to nil so the rest of the
// codebase has a single "is published" signal: Publish != nil.
func (n *Networking) UnmarshalYAML(node *yaml.Node) error {
	var aux struct {
		ExposeToPlatform bool      `yaml:"exposeToPlatform"`
		Publish          yaml.Node `yaml:"publish"`
	}
	if err := node.Decode(&aux); err != nil {
		return err
	}
	publish, err := parsePublishNode(&aux.Publish)
	if err != nil {
		return err
	}
	n.ExposeToPlatform = aux.ExposeToPlatform
	n.Publish = publish
	return nil
}

func parsePublishNode(node *yaml.Node) (*Publish, error) {
	switch node.Kind {
	case 0:
		return nil, nil
	case yaml.ScalarNode:
		if node.Tag == "!!null" {
			return nil, nil
		}
		var enabled bool
		if err := node.Decode(&enabled); err != nil {
			return nil, fmt.Errorf("networking.publish must be a boolean or a mapping with hostPort")
		}
		if !enabled {
			return nil, nil
		}
		return &Publish{}, nil
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			if key := node.Content[i].Value; key != "hostPort" {
				return nil, fmt.Errorf("networking.publish: unknown field %q (only hostPort is valid)", key)
			}
		}
		var p Publish
		if err := node.Decode(&p); err != nil {
			return nil, fmt.Errorf("networking.publish: %w", err)
		}
		return &p, nil
	default:
		return nil, fmt.Errorf("networking.publish must be a boolean or a mapping with hostPort")
	}
}

// ShouldAttachToPlatform reports whether the workload joins the shared
// platform network — declared via exposeToPlatform or implied by publishing.
func (n Networking) ShouldAttachToPlatform() bool {
	return n.ExposeToPlatform || n.Publish != nil
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
