package backends

import (
	"fmt"
)

// DryRunContainerBackend implements ContainerBackend by printing Docker operations.
type DryRunContainerBackend struct {
	Networks map[string]bool
}

func NewDryRunContainerBackend() *DryRunContainerBackend {
	return &DryRunContainerBackend{
		Networks: make(map[string]bool),
	}
}

func (d *DryRunContainerBackend) CreateNetwork(name string) error {
	if d.Networks[name] {
		return nil
	}

	d.Networks[name] = true
	fmt.Printf("[DOCKER] NetworkCreate: name=%s\n", name)
	return nil
}

func (d *DryRunContainerBackend) RemoveNetwork(name string) error {
	fmt.Printf("[DOCKER] NetworkRemove: name=%s\n", name)
	return nil
}

func (d *DryRunContainerBackend) CreateContainer(op CreateContainerOp) error {
	fmt.Printf("[DOCKER] ContainerCreate: name=%s image=%s\n", op.Name, op.Image)
	return nil
}

func (d *DryRunContainerBackend) RemoveContainer(name string) error {
	fmt.Printf("[DOCKER] ContainerRemove: name=%s\n", name)
	return nil
}
