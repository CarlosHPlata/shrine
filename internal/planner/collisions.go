package planner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

type routeKey struct {
	host       string
	pathPrefix string
}

func normalizePrefix(p string) string {
	return strings.TrimRight(p, "/")
}

func appRef(app *manifest.ApplicationManifest) string {
	return app.Metadata.Owner + "/" + app.Metadata.Name
}

// DetectRoutingCollisions checks every route claimed in set but fails only on
// pairs where at least one application is selected by filter (spec 019 FR-010).
func DetectRoutingCollisions(set *ManifestSet, filter Filter) error {
	refToApp := make(map[string]*manifest.ApplicationManifest, len(set.Applications))
	appRefs := make([]string, 0, len(set.Applications))
	for _, app := range set.Applications {
		ref := appRef(app)
		refToApp[ref] = app
		appRefs = append(appRefs, ref)
	}
	sort.Strings(appRefs)

	seen := map[routeKey]string{}
	var errs []string

	addRoute := func(key routeKey, ref string) {
		existing, ok := seen[key]
		if !ok {
			seen[key] = ref
			return
		}
		if existing == ref || !isPairInScope(filter, refToApp[existing], refToApp[ref]) {
			return
		}
		a, b := existing, ref
		if a > b {
			a, b = b, a
		}
		errs = append(errs, fmt.Sprintf(
			"routing collision: host=%q pathPrefix=%q declared by %q and %q",
			key.host, key.pathPrefix, a, b,
		))
	}

	for _, ref := range appRefs {
		routing := refToApp[ref].Spec.Routing
		if routing.Domain != "" {
			addRoute(routeKey{host: routing.Domain, pathPrefix: normalizePrefix(routing.PathPrefix)}, ref)
		}
		for _, alias := range routing.Aliases {
			addRoute(routeKey{host: alias.Host, pathPrefix: normalizePrefix(alias.PathPrefix)}, ref)
		}
	}

	if len(errs) == 0 {
		return nil
	}
	sort.Strings(errs)
	return fmt.Errorf("routing validation failed:\n- %s", strings.Join(errs, "\n- "))
}

func isPairInScope(filter Filter, a, b *manifest.ApplicationManifest) bool {
	return filter.isAppInScope(a) || filter.isAppInScope(b)
}
