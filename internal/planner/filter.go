package planner

import (
	"fmt"
	"sort"
)

// FilterKind selects which subset of a ManifestSet yields deploy steps.
type FilterKind int

const (
	FilterNone FilterKind = iota
	FilterTeam
	FilterApp
	FilterRes
)

// Filter is the value supplied to Plan that scopes step emission.
// The zero value is FilterNone (all manifests).
type Filter struct {
	Kind FilterKind
	Name string
}

func NoFilter() Filter              { return Filter{Kind: FilterNone} }
func ByTeam(name string) Filter     { return Filter{Kind: FilterTeam, Name: name} }
func ByApp(name string) Filter      { return Filter{Kind: FilterApp, Name: name} }
func ByResource(name string) Filter { return Filter{Kind: FilterRes, Name: name} }

// Validate checks whether the filter is satisfiable against the given set.
// A FilterNone always passes. Named filters require both a non-empty name and
// a matching manifest (team owner, app name, or resource name) in the set.
func (f Filter) Validate(set *ManifestSet) error {
	switch f.Kind {
	case FilterNone:
		return nil

	case FilterTeam:
		if f.Name == "" {
			return fmt.Errorf("team filter requires a non-empty name")
		}
		if hasOwner(set, f.Name) {
			return nil
		}
		owners := discoveredOwners(set)
		if len(owners) == 0 {
			return fmt.Errorf("team %q not found: specs directory contains no Application or Resource manifests", f.Name)
		}
		return fmt.Errorf("team %q not found in specs directory: known teams = %v", f.Name, owners)

	case FilterApp:
		if f.Name == "" {
			return fmt.Errorf("application filter requires a non-empty name")
		}
		if _, ok := set.Applications[f.Name]; !ok {
			return fmt.Errorf("application %q not found in manifest set", f.Name)
		}
		return nil

	case FilterRes:
		if f.Name == "" {
			return fmt.Errorf("resource filter requires a non-empty name")
		}
		if _, ok := set.Resources[f.Name]; !ok {
			return fmt.Errorf("resource %q not found in manifest set", f.Name)
		}
		return nil

	default:
		return fmt.Errorf("unknown filter kind: %v", f.Kind)
	}
}

func hasOwner(set *ManifestSet, owner string) bool {
	for _, app := range set.Applications {
		if app.Metadata.Owner == owner {
			return true
		}
	}
	for _, res := range set.Resources {
		if res.Metadata.Owner == owner {
			return true
		}
	}
	return false
}

func discoveredOwners(set *ManifestSet) []string {
	seen := make(map[string]struct{})
	for _, app := range set.Applications {
		seen[app.Metadata.Owner] = struct{}{}
	}
	for _, res := range set.Resources {
		seen[res.Metadata.Owner] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for owner := range seen {
		out = append(out, owner)
	}
	sort.Strings(out)
	return out
}
