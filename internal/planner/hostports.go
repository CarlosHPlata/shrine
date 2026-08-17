package planner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/state"
)

// PortContext carries the host-port knowledge that exists outside the manifest
// set: the gateway's own reserved ports and the persisted allocations from
// earlier deploys. Handlers assemble it from config and the HostPortStore.
type PortContext struct {
	Reserved  []int
	Persisted state.HostPortMap
}

// DetectHostPortCollisions rejects explicit publish claims that collide with
// each other, with a gateway-reserved port, or with another application's
// persisted allocation. All conflicts are reported in one deterministic,
// sorted error; automatic publishers are never flagged. An application
// re-claiming its own persisted port passes.
func DetectHostPortCollisions(set *ManifestSet, ports PortContext) error {
	type claim struct {
		ref  string
		port int
	}

	claims := make([]claim, 0)
	for _, app := range set.Applications {
		publish := app.Spec.Networking.Publish
		if publish == nil || publish.HostPort <= 0 {
			continue
		}
		ref := state.HostPortKey(app.Metadata.Owner, app.Metadata.Name)
		claims = append(claims, claim{ref: ref, port: publish.HostPort})
	}
	sort.Slice(claims, func(i, j int) bool { return claims[i].ref < claims[j].ref })

	reserved := make(map[int]struct{}, len(ports.Reserved))
	for _, p := range ports.Reserved {
		reserved[p] = struct{}{}
	}
	persistedHolder := make(map[int]string, len(ports.Persisted))
	for key, p := range ports.Persisted {
		persistedHolder[p] = key
	}

	seen := make(map[int]string)
	var errs []string

	for _, c := range claims {
		if prior, ok := seen[c.port]; ok && prior != c.ref {
			a, b := prior, c.ref
			if a > b {
				a, b = b, a
			}
			errs = append(errs, fmt.Sprintf(
				"host port collision: port %d declared by %q and %q", c.port, a, b))
		} else {
			seen[c.port] = c.ref
		}

		if _, isReserved := reserved[c.port]; isReserved {
			errs = append(errs, fmt.Sprintf(
				"host port reserved: port %d declared by %q is reserved by the platform gateway", c.port, c.ref))
		}

		if holder, held := persistedHolder[c.port]; held && holder != c.ref {
			errs = append(errs, fmt.Sprintf(
				"host port taken: port %d declared by %q is already allocated to %q", c.port, c.ref, holder))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	sort.Strings(errs)
	return fmt.Errorf("host port validation failed:\n- %s", strings.Join(errs, "\n- "))
}
