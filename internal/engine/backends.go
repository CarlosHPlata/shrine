package engine

import "time"

type VolumeMount struct {
	Name      string
	MountPath string
}

type BindMount struct {
	Source string
	Target string
}

type PortBinding struct {
	HostIP        string // "" binds all interfaces; "127.0.0.1" binds loopback only
	HostPort      string
	ContainerPort string
	Protocol      string
}

// PublishPort is the publish *request* projected from the manifest; the
// container backend resolves it into a concrete PortBinding (allocating a
// host port when HostPort is 0).
type PublishPort struct {
	HostPort      int // 0 = automatic allocation
	ContainerPort int
}

type CreateContainerOp struct {
	Team             string
	Name             string
	Image            string
	Kind             string
	Network          string
	Env              []string
	Volumes          []VolumeMount
	ExposeToPlatform bool
	ImagePullPolicy  string
	RestartPolicy    string
	BindMounts       []BindMount
	PortBindings     []PortBinding
	Publish          *PublishPort
	// Image stays the reference as the manifest wrote it, because the
	// deployment record keeps that form. ResolvedRef is the pullable reference
	// the container is created from and ImageID the config-hash input; the
	// engine fills both from the pre-pass, the Traefik plugin leaves them
	// empty and CreateContainer resolves on its own.
	ResolvedRef string
	ImageID     string
}

type RemoveContainerOp struct {
	Team string
	Name string
}

type ContainerInfo struct {
	Running bool
	Status  string
	ImageID string
	Image   string // the reference the container was created from, as the runtime reports it
}

type ResolveImageOp struct {
	Team            string
	Name            string
	Kind            string
	Image           string
	ImagePullPolicy string
}

// ResolvedImage keeps the pullable reference and the local image id apart:
// Ref is what a container is created from and what a later pin can pull;
// ImageID is the config-hash input and is not pullable from any registry.
// Requested is the tag reference a pin was resolved from, kept because Ref
// is a digest reference under Pinned and the readable tag would be lost.
type ResolvedImage struct {
	Ref       string
	Digest    string
	ImageID   string
	Source    string
	Requested string
	PinnedAt  time.Time
}

const (
	ImageSourceManifest = "manifest"
	ImageSourceResolved = "resolved"
	ImageSourcePinned   = "pinned"
)

type ContainerBackend interface {
	CreateNetwork(name string) error
	RemoveNetwork(name string) error
	CreateContainer(op CreateContainerOp) error
	RemoveContainer(op RemoveContainerOp) error
	CreatePlatformNetwork() error
	InspectContainer(containerID string) (ContainerInfo, error)
	ResolveImage(op ResolveImageOp) (ResolvedImage, error)
}

type AliasRoute struct {
	Host        string
	PathPrefix  string
	StripPrefix bool
	TLS         bool
}

type WriteRouteOp struct {
	Team             string
	Domain           string
	ServiceName      string
	ServicePort      int
	PathPrefix       string
	AdditionalRoutes []AliasRoute
}

type RoutingBackend interface {
	WriteRoute(op WriteRouteOp) error
	RemoveRoute(team string, host string) error
	Finalize() error
}

type WriteRecordOp struct {
	Team       string
	Name       string
	RecordType string
	Value      string
}

type DNSBackend interface {
	WriteRecord(op WriteRecordOp) error
	RemoveRecord(team string, name string) error
}
