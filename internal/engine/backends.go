package engine

type VolumeMount struct {
	Name      string
	MountPath string
}

type CreateContainerOp struct {
	Team    string
	Name    string
	Image   string
	Kind    string
	Network string
	Env     []string
	Volumes []VolumeMount
}

type RemoveContainerOp struct {
	Team string
	Name string
}

type ContainerBackend interface {
	CreateNetwork(name string) error
	RemoveNetwork(name string) error
	CreateContainer(op CreateContainerOp) error
	RemoveContainer(op RemoveContainerOp) error
}

type WriteRouteOp struct {
	Team        string
	Domain      string
	ServiceName string
	ServicePort int
	PathPrefix  string
}

type RoutingBackend interface {
	WriteRoute(op WriteRouteOp) error
	RemoveRoute(team string, host string) error
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
