package handler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

type teamedDeployment struct {
	Team       string
	Deployment state.Deployment
}

func collectAllDeployments(store *state.Store) ([]teamedDeployment, error) {
	teams, err := store.Teams.ListTeams()
	if err != nil {
		return nil, fmt.Errorf("listing teams: %w", err)
	}
	var all []teamedDeployment
	for _, team := range teams {
		deployments, err := store.Deployments.List(team.Metadata.Name)
		if err != nil {
			return nil, fmt.Errorf("listing deployments for team %q: %w", team.Metadata.Name, err)
		}
		for _, d := range deployments {
			all = append(all, teamedDeployment{Team: team.Metadata.Name, Deployment: d})
		}
	}
	return all, nil
}

func collectTeamDeployments(team string, store *state.Store) ([]teamedDeployment, error) {
	deployments, err := store.Deployments.List(team)
	if err != nil {
		return nil, fmt.Errorf("listing deployments for team %q: %w", team, err)
	}
	result := make([]teamedDeployment, len(deployments))
	for i, d := range deployments {
		result[i] = teamedDeployment{Team: team, Deployment: d}
	}
	return result, nil
}

func deploymentsForTeamOrAll(team string, store *state.Store) ([]teamedDeployment, error) {
	if team == "" {
		return collectAllDeployments(store)
	}
	return collectTeamDeployments(team, store)
}

func filterByKind(deployments []teamedDeployment, kind string) []teamedDeployment {
	var filtered []teamedDeployment
	for _, d := range deployments {
		if d.Deployment.Kind == kind {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

func shortContainerID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func printDeploymentsTable(deployments []teamedDeployment) {
	fmt.Printf("%-20s %-30s %-15s %-15s\n", "TEAM", "NAME", "KIND", "CONTAINER ID")
	fmt.Println(strings.Repeat("-", 83))
	for _, td := range deployments {
		fmt.Printf("%-20s %-30s %-15s %-15s\n",
			td.Team,
			td.Deployment.Name,
			td.Deployment.Kind,
			shortContainerID(td.Deployment.ContainerID),
		)
	}
}

func ListApplications(team string, store *state.Store) error {
	deployments, err := deploymentsForTeamOrAll(team, store)
	if err != nil {
		return err
	}
	apps := filterByKind(deployments, manifest.ApplicationKind)
	if len(apps) == 0 {
		fmt.Println("No applications deployed.")
		return nil
	}
	printDeploymentsTable(apps)
	return nil
}

func ListResources(team string, store *state.Store) error {
	deployments, err := deploymentsForTeamOrAll(team, store)
	if err != nil {
		return err
	}
	resources := filterByKind(deployments, manifest.ResourceKind)
	if len(resources) == 0 {
		fmt.Println("No resources deployed.")
		return nil
	}
	printDeploymentsTable(resources)
	return nil
}

func ListDeployed(team string, store *state.Store) error {
	deployments, err := deploymentsForTeamOrAll(team, store)
	if err != nil {
		return err
	}
	if len(deployments) == 0 {
		fmt.Println("No deployments found.")
		return nil
	}
	printDeploymentsTable(deployments)
	return nil
}

func DescribeApplication(team, name string, store *state.Store) error {
	return describeDeployment(team, name, manifest.ApplicationKind, store)
}

func DescribeResource(team, name string, store *state.Store) error {
	return describeDeployment(team, name, manifest.ResourceKind, store)
}

func describeDeployment(team, name, kind string, store *state.Store) error {
	if team != "" {
		// Explicit team: search only within that team.
		deployments, err := store.Deployments.List(team)
		if err != nil {
			return fmt.Errorf("listing deployments for team %q: %w", team, err)
		}
		for _, d := range deployments {
			if d.Name == name && d.Kind == kind {
				printDeploymentDetail(team, d)
				return nil
			}
		}
		return fmt.Errorf("%s %q not found in team %q", kind, name, team)
	}

	// No team specified: search all teams and disambiguate.
	all, err := collectAllDeployments(store)
	if err != nil {
		return err
	}

	var matches []teamedDeployment
	for _, td := range all {
		if td.Deployment.Name == name && td.Deployment.Kind == kind {
			matches = append(matches, td)
		}
	}

	switch len(matches) {
	case 0:
		return fmt.Errorf("%s %q not found in any team", kind, name)
	case 1:
		printDeploymentDetail(matches[0].Team, matches[0].Deployment)
		return nil
	default:
		teamNames := make([]string, len(matches))
		for i, m := range matches {
			teamNames[i] = m.Team
		}
		return fmt.Errorf("ambiguous: %s %q found in teams [%s], use --team to disambiguate",
			kind, name, strings.Join(teamNames, ", "))
	}
}

// DeleteApplicationOptions parameterizes DeleteApplication. Team is optional
// (kubectl-style: all teams are searched, ambiguity is an error). DryRun
// prints what would be released without writing.
type DeleteApplicationOptions struct {
	Name   string
	Team   string
	DryRun bool
}

// DeleteApplication forgets an application: it releases the published
// host-port allocation and drops the stale deployment record. Docker is
// authoritative — while the container still exists the delete refuses and
// points at teardown.
func DeleteApplication(store *state.Store, container engine.ContainerBackend, opts DeleteApplicationOptions) error {
	team, err := resolveDeleteTeam(store, opts.Name, opts.Team)
	if err != nil {
		return err
	}
	if team == "" {
		fmt.Printf("Nothing to delete for application %q.\n", opts.Name)
		return nil
	}
	ref := team + "/" + opts.Name

	if container != nil {
		if _, err := container.InspectContainer(team + "." + opts.Name); err == nil {
			return fmt.Errorf("application %q still has a container; run \"shrine teardown %s\" first", ref, team)
		}
	}

	port, portErr := store.HostPorts.GetHostPort(team, opts.Name)
	hasPort := portErr == nil
	record := findApplicationRecord(store, team, opts.Name)

	if opts.DryRun {
		if hasPort {
			fmt.Printf("[dry-run] would release host port %d for %s\n", port, ref)
		}
		if record {
			fmt.Printf("[dry-run] would remove deployment record for %s\n", ref)
		}
		if !hasPort && !record {
			fmt.Printf("[dry-run] nothing to delete for application %q in team %q\n", opts.Name, team)
		}
		return nil
	}

	if hasPort {
		if err := store.HostPorts.ReleaseHostPort(team, opts.Name); err != nil {
			return fmt.Errorf("releasing host port for %s: %w", ref, err)
		}
		fmt.Printf("Released host port %d for %s.\n", port, ref)
	}
	if record {
		if err := store.Deployments.Remove(team, opts.Name); err != nil {
			return fmt.Errorf("removing deployment record for %s: %w", ref, err)
		}
		fmt.Printf("Removed deployment record for %s.\n", ref)
	}
	if !hasPort && !record {
		fmt.Printf("Nothing to delete for application %q in team %q.\n", opts.Name, team)
	}
	return nil
}

// resolveDeleteTeam returns the team owning the application, searching every
// team's allocations and deployment records when none was given. An empty
// result with a nil error means the application is unknown everywhere.
func resolveDeleteTeam(store *state.Store, name, team string) (string, error) {
	if team != "" {
		return team, nil
	}

	candidates := map[string]bool{}
	if store.HostPorts != nil {
		ports, err := store.HostPorts.ListHostPorts()
		if err != nil {
			return "", fmt.Errorf("listing host port allocations: %w", err)
		}
		for key := range ports {
			owner, app, found := strings.Cut(key, "/")
			if found && app == name {
				candidates[owner] = true
			}
		}
	}
	all, err := collectAllDeployments(store)
	if err != nil {
		return "", err
	}
	for _, td := range all {
		if td.Deployment.Name == name && td.Deployment.Kind == manifest.ApplicationKind {
			candidates[td.Team] = true
		}
	}

	switch len(candidates) {
	case 0:
		return "", nil
	case 1:
		for team := range candidates {
			return team, nil
		}
	}
	names := make([]string, 0, len(candidates))
	for team := range candidates {
		names = append(names, team)
	}
	sort.Strings(names)
	return "", fmt.Errorf("ambiguous: application %q found in teams [%s], use --team to disambiguate",
		name, strings.Join(names, ", "))
}

func findApplicationRecord(store *state.Store, team, name string) bool {
	deployments, err := store.Deployments.List(team)
	if err != nil {
		return false
	}
	for _, d := range deployments {
		if d.Name == name && d.Kind == manifest.ApplicationKind {
			return true
		}
	}
	return false
}

func printDeploymentDetail(team string, d state.Deployment) {
	hashPreview := d.ConfigHash
	if len(hashPreview) > 16 {
		hashPreview = hashPreview[:16] + "..."
	}
	fmt.Printf("Name:         %s\n", d.Name)
	fmt.Printf("Team:         %s\n", team)
	fmt.Printf("Kind:         %s\n", d.Kind)
	fmt.Printf("Container ID: %s\n", d.ContainerID)
	fmt.Printf("Config Hash:  %s\n", hashPreview)
}
