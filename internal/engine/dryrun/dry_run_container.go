package dryrun

import (
	"fmt"
	"github.com/CarlosHPlata/shrine/internal/engine"
)

// DryRunContainerBackend implements ContainerBackend by printing Docker operations.
type DryRunContainerBackend struct{}

func (d *DryRunContainerBackend) CreateNetwork(name string) error {
	fmt.Printf("[DOCKER] NetworkCreate: name=%s\n", name)
	return nil
}

func (d *DryRunContainerBackend) RemoveNetwork(name string) error {
	fmt.Printf("[DOCKER] NetworkRemove: name=%s\n", name)
	return nil
}

func (d *DryRunContainerBackend) CreateContainer(op engine.CreateContainerOp) error {
	fmt.Printf("[DOCKER] ContainerCreate: name=%s image=%s\n", op.Name, op.Image)
	return nil
}

func (d *DryRunContainerBackend) RemoveContainer(name string) error {
	fmt.Printf("[DOCKER] ContainerRemove: name=%s\n", name)
	return nil
}
