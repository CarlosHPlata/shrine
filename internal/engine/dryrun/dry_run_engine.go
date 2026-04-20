package dryrun

import (
	"io"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/resolver"
)

func NewDryRunEngine(out io.Writer) *engine.Engine {
	return &engine.Engine{
		Container: NewDryRunContainerBackend(out),
		Routing:   &DryRunRoutingBackend{Out: out},
		DNS:       &DryRunDNSBackend{Out: out},
		Resolver:  resolver.NewDryRunResolver(),
	}
}
