package handler

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

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

const deploymentRowFormat = "%-20s %-30s %-15s %-40s %-15s\n"

func valueOrUnknown(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// loadImagePins reads every pin once for a listing; a store without a pin
// store (hand-built in tests) simply has none.
func loadImagePins(store *state.Store) (map[string]state.ImagePin, error) {
	if store.ImagePins == nil {
		return map[string]state.ImagePin{}, nil
	}
	pins, err := store.ImagePins.ListAll()
	if err != nil {
		return nil, fmt.Errorf("listing image pins: %w", err)
	}
	return pins, nil
}

// pinFor finds the pin a deployment record points at. The pin key carries no
// kind, so the kind is checked here.
func pinFor(team string, d state.Deployment, pins map[string]state.ImagePin) (state.ImagePin, bool) {
	pin, ok := pins[state.ImagePinKey(team, d.Name)]
	if !ok || pin.Kind != d.Kind {
		return state.ImagePin{}, false
	}
	return pin, true
}

func readablePin(pin state.ImagePin) string {
	return manifest.ReadableVersion(pin.Requested, manifest.DigestOf(pin.Pinned))
}

// versionCell is the readable form of the pin for a Pinned record and the
// reference the manifest named for every other one (design section 4.7).
func versionCell(team string, d state.Deployment, pins map[string]state.ImagePin) string {
	if d.Policy == manifest.ImagePullPolicyPinned {
		if pin, ok := pinFor(team, d, pins); ok {
			return readablePin(pin)
		}
	}
	return valueOrUnknown(d.Image)
}

func formatDeploymentsTable(deployments []teamedDeployment, pins map[string]state.ImagePin) string {
	var b strings.Builder
	header := fmt.Sprintf(deploymentRowFormat, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")
	b.WriteString(header)
	b.WriteString(strings.Repeat("-", len(strings.TrimSuffix(header, "\n"))))
	b.WriteString("\n")
	for _, td := range deployments {
		fmt.Fprintf(&b, deploymentRowFormat,
			td.Team,
			td.Deployment.Name,
			td.Deployment.Kind,
			versionCell(td.Team, td.Deployment, pins),
			shortContainerID(td.Deployment.ContainerID),
		)
	}
	return b.String()
}

func printDeploymentsTable(deployments []teamedDeployment, store *state.Store) error {
	pins, err := loadImagePins(store)
	if err != nil {
		return err
	}
	fmt.Print(formatDeploymentsTable(deployments, pins))
	return nil
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
	return printDeploymentsTable(apps, store)
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
	return printDeploymentsTable(resources, store)
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
	return printDeploymentsTable(deployments, store)
}

func DescribeApplication(team, name string, store *state.Store, backend engine.ContainerBackend) error {
	return describeDeployment(team, name, manifest.ApplicationKind, store, backend)
}

func DescribeResource(team, name string, store *state.Store, backend engine.ContainerBackend) error {
	return describeDeployment(team, name, manifest.ResourceKind, store, backend)
}

// describeDeployment never fails because of the backend: the record is the
// source of every line but the running image, which degrades on its own.
func describeDeployment(team, name, kind string, store *state.Store, backend engine.ContainerBackend) error {
	td, err := findDeployment(team, name, kind, store)
	if err != nil {
		return err
	}
	detail, err := gatherDeploymentDetail(td, store, backend)
	if err != nil {
		return err
	}
	fmt.Print(formatDeploymentDetail(detail))
	return nil
}

func findDeployment(team, name, kind string, store *state.Store) (teamedDeployment, error) {
	if team != "" {
		deployments, err := store.Deployments.List(team)
		if err != nil {
			return teamedDeployment{}, fmt.Errorf("listing deployments for team %q: %w", team, err)
		}
		for _, d := range deployments {
			if d.Name == name && d.Kind == kind {
				return teamedDeployment{Team: team, Deployment: d}, nil
			}
		}
		return teamedDeployment{}, fmt.Errorf("%s %q not found in team %q", kind, name, team)
	}

	all, err := collectAllDeployments(store)
	if err != nil {
		return teamedDeployment{}, err
	}
	var matches []teamedDeployment
	for _, td := range all {
		if td.Deployment.Name == name && td.Deployment.Kind == kind {
			matches = append(matches, td)
		}
	}
	switch len(matches) {
	case 0:
		return teamedDeployment{}, fmt.Errorf("%s %q not found in any team", kind, name)
	case 1:
		return matches[0], nil
	default:
		teamNames := make([]string, len(matches))
		for i, m := range matches {
			teamNames[i] = m.Team
		}
		return teamedDeployment{}, fmt.Errorf("ambiguous: %s %q found in teams [%s], use --team to disambiguate",
			kind, name, strings.Join(teamNames, ", "))
	}
}

// deploymentDetail is everything the describe block prints: the record, the
// pin it points at when it is Pinned, and the running image already rendered.
type deploymentDetail struct {
	Team         string
	Deployment   state.Deployment
	Pin          state.ImagePin
	HasPin       bool
	RunningImage string
}

func gatherDeploymentDetail(td teamedDeployment, store *state.Store, backend engine.ContainerBackend) (deploymentDetail, error) {
	pin, hasPin, err := lookupPin(store, td.Team, td.Deployment)
	if err != nil {
		return deploymentDetail{}, err
	}
	return deploymentDetail{
		Team:         td.Team,
		Deployment:   td.Deployment,
		Pin:          pin,
		HasPin:       hasPin,
		RunningImage: runningImage(backend, td.Deployment.ContainerID),
	}, nil
}

// lookupPin reads the pin only for a Pinned record; a missing pin is shown,
// not failed, because the record is what proves the artifact is deployed.
func lookupPin(store *state.Store, team string, d state.Deployment) (state.ImagePin, bool, error) {
	if d.Policy != manifest.ImagePullPolicyPinned || store.ImagePins == nil {
		return state.ImagePin{}, false, nil
	}
	pin, err := store.ImagePins.Get(team, d.Name)
	if errors.Is(err, state.ErrImagePinNotFound) {
		return state.ImagePin{}, false, nil
	}
	if err != nil {
		return state.ImagePin{}, false, fmt.Errorf("reading image pin for %s/%s: %w", team, d.Name, err)
	}
	if pin.Kind != d.Kind {
		return state.ImagePin{}, false, nil
	}
	return pin, true, nil
}

// runningImage is read live because Docker, not the record, knows what a
// container was created from; without Docker the line degrades alone.
func runningImage(backend engine.ContainerBackend, containerID string) string {
	if backend == nil {
		return "unavailable (no container runtime)"
	}
	info, err := backend.InspectContainer(containerID)
	if err != nil {
		return fmt.Sprintf("unavailable (%v)", err)
	}
	return valueOrUnknown(info.Image)
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
	pin, hasPin := findImagePin(store, team, opts.Name)
	record := findApplicationRecord(store, team, opts.Name)
	nothingHeld := !hasPort && !hasPin && !record

	if opts.DryRun {
		if hasPort {
			fmt.Printf("[dry-run] would release host port %d for %s\n", port, ref)
		}
		if hasPin {
			fmt.Printf("[dry-run] would release image pin %s for %s\n", pin.Pinned, ref)
		}
		if record {
			fmt.Printf("[dry-run] would remove deployment record for %s\n", ref)
		}
		if nothingHeld {
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
	if hasPin {
		if err := store.ImagePins.Release(team, opts.Name); err != nil {
			return fmt.Errorf("releasing image pin for %s: %w", ref, err)
		}
		fmt.Printf("Released image pin for %s.\n", ref)
	}
	if record {
		if err := store.Deployments.Remove(team, opts.Name); err != nil {
			return fmt.Errorf("removing deployment record for %s: %w", ref, err)
		}
		fmt.Printf("Removed deployment record for %s.\n", ref)
	}
	if nothingHeld {
		fmt.Printf("Nothing to delete for application %q in team %q.\n", opts.Name, team)
	}
	return nil
}

// findImagePin reports the application's pin when the store has one; a
// store without pins (older callers, partial test stores) holds none.
func findImagePin(store *state.Store, team, name string) (state.ImagePin, bool) {
	if store.ImagePins == nil {
		return state.ImagePin{}, false
	}
	pin, err := store.ImagePins.Get(team, name)
	return pin, err == nil
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
	if store.ImagePins != nil {
		pins, err := store.ImagePins.ListAll()
		if err != nil {
			return "", fmt.Errorf("listing image pins: %w", err)
		}
		for key, pin := range pins {
			owner, app, found := strings.Cut(key, "/")
			if found && app == name && pin.Kind == manifest.ApplicationKind {
				candidates[owner] = true
			}
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

func pinLine(d deploymentDetail) string {
	if d.Deployment.Policy != manifest.ImagePullPolicyPinned {
		return ""
	}
	if !d.HasPin {
		return "Pinned:       -\n"
	}
	return fmt.Sprintf("Pinned:       %s (%s, %s)\n", d.Pin.Pinned, readablePin(d.Pin), d.Pin.PinnedAt.UTC().Format(time.DateOnly))
}

func formatDeploymentDetail(d deploymentDetail) string {
	hashPreview := d.Deployment.ConfigHash
	if len(hashPreview) > 16 {
		hashPreview = hashPreview[:16] + "..."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Name:         %s\n", d.Deployment.Name)
	fmt.Fprintf(&b, "Team:         %s\n", d.Team)
	fmt.Fprintf(&b, "Kind:         %s\n", d.Deployment.Kind)
	fmt.Fprintf(&b, "Image:        %s\n", valueOrUnknown(d.Deployment.Image))
	fmt.Fprintf(&b, "Pull policy:  %s\n", valueOrUnknown(d.Deployment.Policy))
	b.WriteString(pinLine(d))
	fmt.Fprintf(&b, "Running image: %s\n", d.RunningImage)
	fmt.Fprintf(&b, "Container ID: %s\n", d.Deployment.ContainerID)
	fmt.Fprintf(&b, "Config Hash:  %s\n", hashPreview)
	return b.String()
}
