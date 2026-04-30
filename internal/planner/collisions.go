package planner

import (
	"fmt"
	"sort"
	"strings"
)

type routeKey struct {
	host       string
	pathPrefix string
}

func normalizePrefix(p string) string {
	return strings.TrimRight(p, "/")
}

func DetectRoutingCollisions(set *ManifestSet) error {
	type record struct {
		appRef string
		key    routeKey
	}

	// Collect all (appRef, routeKey) pairs in deterministic order.
	appRefs := make([]string, 0, len(set.Applications))
	for name := range set.Applications {
		app := set.Applications[name]
		appRefs = append(appRefs, app.Metadata.Owner+"/"+app.Metadata.Name)
	}
	sort.Strings(appRefs)

	// Build a map from appRef to the app for sorted iteration.
	refToApp := make(map[string]*struct {
		domain     string
		pathPrefix string
		aliases    []struct {
			host       string
			pathPrefix string
		}
	}, len(set.Applications))

	for _, app := range set.Applications {
		ref := app.Metadata.Owner + "/" + app.Metadata.Name
		entry := &struct {
			domain     string
			pathPrefix string
			aliases    []struct {
				host       string
				pathPrefix string
			}
		}{
			domain:     app.Spec.Routing.Domain,
			pathPrefix: normalizePrefix(app.Spec.Routing.PathPrefix),
		}
		for _, alias := range app.Spec.Routing.Aliases {
			entry.aliases = append(entry.aliases, struct {
				host       string
				pathPrefix string
			}{host: alias.Host, pathPrefix: normalizePrefix(alias.PathPrefix)})
		}
		refToApp[ref] = entry
	}

	seen := map[routeKey]string{}
	var errs []string

	addRoute := func(key routeKey, appRef string) {
		if existing, ok := seen[key]; ok {
			if existing != appRef {
				a, b := existing, appRef
				if a > b {
					a, b = b, a
				}
				errs = append(errs, fmt.Sprintf(
					"routing collision: host=%q pathPrefix=%q declared by %q and %q",
					key.host, key.pathPrefix, a, b,
				))
			}
		} else {
			seen[key] = appRef
		}
	}

	for _, ref := range appRefs {
		entry := refToApp[ref]
		if entry.domain != "" {
			addRoute(routeKey{host: entry.domain, pathPrefix: entry.pathPrefix}, ref)
		}
		for _, alias := range entry.aliases {
			addRoute(routeKey{host: alias.host, pathPrefix: alias.pathPrefix}, ref)
		}
	}

	if len(errs) == 0 {
		return nil
	}
	sort.Strings(errs)
	return fmt.Errorf("routing validation failed:\n- %s", strings.Join(errs, "\n- "))
}
