package dryrun

import (
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/engine"
)

// DryRunContainerBackend implements ContainerBackend by printing Docker operations.
type DryRunContainerBackend struct {
	Out      io.Writer
	Networks map[string]bool
}

func NewDryRunContainerBackend(out io.Writer) *DryRunContainerBackend {
	return &DryRunContainerBackend{
		Out:      out,
		Networks: make(map[string]bool),
	}
}

func (d *DryRunContainerBackend) CreateNetwork(name string) error {
	if d.Networks[name] {
		return nil
	}

	d.Networks[name] = true
	fmt.Fprintf(d.Out, "[DOCKER] NetworkCreate: name=%s\n", name)
	return nil
}

func (d *DryRunContainerBackend) RemoveNetwork(name string) error {
	fmt.Fprintf(d.Out, "[DOCKER] NetworkRemove: name=%s\n", name)
	return nil
}

func (d *DryRunContainerBackend) CreateContainer(op engine.CreateContainerOp) error {
	fmt.Fprintf(d.Out, "[DOCKER] ContainerCreate: name=%s.%s image=%s\n", op.Team, op.Name, op.Image)
	return nil
}

func (d *DryRunContainerBackend) RemoveContainer(name string) error {
	fmt.Fprintf(d.Out, "[DOCKER] ContainerRemove: name=%s\n", name)
	return nil
}
