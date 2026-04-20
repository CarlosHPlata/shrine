package dryrun

import (
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/engine"
)

// DryRunRoutingBackend implements RoutingBackend by printing Traefik route operations.
type DryRunRoutingBackend struct {
	Out io.Writer
}

func (d *DryRunRoutingBackend) WriteRoute(op engine.WriteRouteOp) error {
	fmt.Fprintf(d.Out, "[ROUTE]  WriteRoute: domain=%s → %s:%d\n", op.Domain, op.ServiceName, op.ServicePort)
	return nil
}

func (d *DryRunRoutingBackend) RemoveRoute(team string, domain string) error {
	fmt.Fprintf(d.Out, "[ROUTE]  RemoveRoute: domain=%s (team=%s)\n", domain, team)
	return nil
}
