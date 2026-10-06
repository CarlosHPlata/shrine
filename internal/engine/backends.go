package engine

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
	// ImageID is filled by the engine from the pre-pass so CreateContainer
	// does not resolve the image a second time; the Traefik plugin leaves it
	// empty and keeps resolving on its own.
	ImageID string
}

type RemoveContainerOp struct {
	Team string
	Name string
}

type ContainerInfo struct {
	Running bool
	Status  string
	ImageID string
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
type ResolvedImage struct {
	Ref     string
	Digest  string
	ImageID string
	Source  string
}

const ImageSourceManifest = "manifest"

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
