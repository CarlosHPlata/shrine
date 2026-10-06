package dryrun

import (
	"io"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/resolver"
	"github.com/CarlosHPlata/shrine/internal/state"
)

// NewDryRunEngine builds a print-only engine. hostPorts and pins are
// read-only snapshots of persisted state so the preview can show already-held
// automatic ports and recorded image pins without ever touching the stores.
func NewDryRunEngine(out io.Writer, hostPorts state.HostPortMap, pins map[string]state.ImagePin) *engine.Engine {
	container := NewDryRunContainerBackend(out)
	container.HostPorts = hostPorts
	container.Pins = pins
	return &engine.Engine{
		Container: container,
		Routing:   &DryRunRoutingBackend{Out: out},
		DNS:       &DryRunDNSBackend{Out: out},
		Resolver:  resolver.NewDryRunResolver(),
	}
}
