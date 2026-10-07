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
	ImagePullPolicyPinned       = "Pinned"
)

// IsKnownPullPolicy reports whether policy is one of the three accepted values.
func IsKnownPullPolicy(policy string) bool {
	switch policy {
	case ImagePullPolicyAlways, ImagePullPolicyIfNotPresent, ImagePullPolicyPinned:
		return true
	}
	return false
}

// IsManifestOwnedPolicy reports whether the manifest, not Shrine, owns the
// version under policy.
func IsManifestOwnedPolicy(policy string) bool {
	return policy == ImagePullPolicyAlways || policy == ImagePullPolicyIfNotPresent
}

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

// EffectivePullPolicy returns the declared policy, else the derived rule:
// Always for latest or no tag, IfNotPresent for any other tag or a digest.
func EffectivePullPolicy(image string, declared string) string {
	return EffectivePullPolicyWithDefault(image, declared, "")
}

// EffectivePullPolicyWithDefault applies the precedence of PRD R-04: the
// manifest's own field, then the configuration default, then the derived rule.
func EffectivePullPolicyWithDefault(image, declared, dflt string) string {
	if declared != "" {
		return declared
	}
	if dflt != "" {
		return dflt
	}
	if IsDigestReference(image) {
		return ImagePullPolicyIfNotPresent
	}
	tag := TagOf(image)
	if tag == "" || tag == "latest" {
		return ImagePullPolicyAlways
	}
	return ImagePullPolicyIfNotPresent
}

// TagOf returns the tag of an image reference, or "" when it has none. A
// colon separates the tag only after the last slash, so a registry port is
// never mistaken for one; a digest suffix is cut first.
func TagOf(ref string) string {
	ref, _, _ = strings.Cut(ref, "@")
	slash := strings.LastIndex(ref, "/")
	if colon := strings.LastIndex(ref, ":"); colon > slash {
		return ref[colon+1:]
	}
	return ""
}

// IsDigestReference reports whether ref names an exact version (repo@sha256:…).
func IsDigestReference(ref string) bool {
	return strings.Contains(ref, "@")
}

// DigestOf returns the exact version part of a digest reference
// (repo@sha256:… yields sha256:…), or "" when the reference has none.
func DigestOf(ref string) string {
	_, digest, _ := strings.Cut(ref, "@")
	return digest
}

// ReadableVersion is a pin as a person reads it: the tag it was resolved
// from and a short exact version, or the short exact version alone when the
// request was itself a digest (design section 3.5).
func ReadableVersion(requested, digest string) string {
	if IsDigestReference(requested) {
		return ShortDigest(digest)
	}
	tag := TagOf(requested)
	if tag == "" {
		tag = "latest"
	}
	return tag + "@" + ShortDigest(digest)
}

// ShortDigest keeps twelve hex characters, the length container ids are
// shortened to elsewhere (design TD-11).
func ShortDigest(digest string) string {
	digest = strings.TrimPrefix(digest, "sha256:")
	if len(digest) <= 12 {
		return digest
	}
	return digest[:12]
}
