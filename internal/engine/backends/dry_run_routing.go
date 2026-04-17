package backends

import (
	"fmt"
)

// DryRunRoutingBackend implements RoutingBackend by printing Traefik route operations.
type DryRunRoutingBackend struct{}

func (d *DryRunRoutingBackend) WriteRoute(op WriteRouteOp) error {
	fmt.Printf("[ROUTE]  WriteRoute: domain=%s → %s:%d\n", op.Domain, op.ServiceName, op.ServicePort)
	return nil
}

func (d *DryRunRoutingBackend) RemoveRoute(team string, domain string) error {
	fmt.Printf("[ROUTE]  RemoveRoute: domain=%s (team=%s)\n", domain, team)
	return nil
}
