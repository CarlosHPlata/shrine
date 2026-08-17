package dryrun

import (
	"io"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/resolver"
	"github.com/CarlosHPlata/shrine/internal/state"
)

// NewDryRunEngine builds a print-only engine. hostPorts is a read-only
// snapshot of persisted allocations so the preview can show already-held
// automatic ports without ever touching the store.
func NewDryRunEngine(out io.Writer, hostPorts state.HostPortMap) *engine.Engine {
	container := NewDryRunContainerBackend(out)
	container.HostPorts = hostPorts
	return &engine.Engine{
		Container: container,
		Routing:   &DryRunRoutingBackend{Out: out},
		DNS:       &DryRunDNSBackend{Out: out},
		Resolver:  resolver.NewDryRunResolver(),
	}
}
