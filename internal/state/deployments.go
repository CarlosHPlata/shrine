package state

type Deployment struct {
	Kind        string
	Name        string
	ContainerID string
}

type DeploymentStore interface {
	Record(team string, deployment Deployment) error
	Remove(team string, name string) error
	List(team string) ([]Deployment, error)
}
