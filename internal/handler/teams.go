package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

const teamSkeleton = `apiVersion: shrine/v1
kind: Team
metadata:
  name: %s
spec:
  displayName: "%s"
  contact: "admin@%s.com"
  quotas:
    maxApps: 2
    maxResources: 2
  registryUser: %s
`

// GenerateTeam creates a skeleton team manifest YAML file in the given directory.
func GenerateTeam(name, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating directory %q: %w", outputDir, err)
	}

	path := filepath.Join(outputDir, name+".yml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("team manifest already exists at %q", path)
	}

	content := fmt.Sprintf(teamSkeleton, name, strings.ToUpper(name[:1])+name[1:], name, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing team manifest: %w", err)
	}

	fmt.Printf("Created team manifest: %s\n", path)
	fmt.Println("Run 'shrine apply teams' to register it in the platform state.")
	return nil
}

// CreateTeam parses a single team manifest file and saves it to state.
func CreateTeam(filepath string, store state.TeamStore) error {
	m, err := manifest.Parse(filepath)
	if err != nil {
		return fmt.Errorf("parsing manifest %q: %w", filepath, err)
	}

	if m.Team == nil {
		return fmt.Errorf("file %q is not a Team manifest (kind: %s)", filepath, m.Kind)
	}

	if err := store.SaveTeam(m.Team); err != nil {
		return fmt.Errorf("saving team to state: %w", err)
	}

	fmt.Printf("Created team %q in state.\n", m.Team.Metadata.Name)
	return nil
}

// ApplyTeams scans a directory for team manifests and syncs them all to state.
func ApplyTeams(manifestDir string, store state.TeamStore) error {
	files, err := filepath.Glob(filepath.Join(manifestDir, "*.yml"))
	if err != nil {
		return fmt.Errorf("searching for manifests: %w", err)
	}

	if len(files) == 0 {
		fmt.Printf("No team manifests found in %q directory.\n", manifestDir)
		return nil
	}

	count := 0
	for _, file := range files {
		m, err := manifest.Parse(file)
		if err != nil {
			fmt.Printf("Error parsing %s: %v\n", file, err)
			continue
		}

		if m.Team == nil {
			fmt.Printf("Skipping %s: not a Team manifest\n", file)
			continue
		}

		if err := store.SaveTeam(m.Team); err != nil {
			fmt.Printf("Error saving team %s to state: %v\n", m.Team.Metadata.Name, err)
			continue
		}
		fmt.Printf("Synced team: %s\n", m.Team.Metadata.Name)
		count++
	}

	fmt.Printf("Successfully synced %d teams to state.\n", count)
	return nil
}

// ListTeams displays all registered teams in a table format.
func ListTeams(store state.TeamStore) error {
	teams, err := store.ListTeams()
	if err != nil {
		return fmt.Errorf("listing teams: %w", err)
	}

	if len(teams) == 0 {
		fmt.Println("No teams registered. Run 'shrine apply teams' to sync manifests.")
		return nil
	}

	fmt.Printf("%-20s %-30s %-10s %-10s\n", "NAME", "DISPLAY NAME", "MAX APPS", "MAX RESOURCES")
	fmt.Println(strings.Repeat("-", 75))
	for _, t := range teams {
		fmt.Printf("%-20s %-30s %-10d %-10d\n",
			t.Metadata.Name,
			t.Spec.DisplayName,
			t.Spec.Quotas.MaxApps,
			t.Spec.Quotas.MaxResources,
		)
	}

	return nil
}

// DescribeTeam displays detailed info about a single team from state.
func DescribeTeam(name string, store *state.Store) error {
	team, err := store.Teams.LoadTeam(name)
	if err != nil {
		return err
	}

	fmt.Printf("Team: %s\n", team.Metadata.Name)
	fmt.Printf("Display Name: %s\n", team.Spec.DisplayName)
	fmt.Printf("Contact: %s\n", team.Spec.Contact)
	fmt.Printf("Registry User: %s\n", team.Spec.RegistryUser)
	fmt.Println("Quotas:")
	fmt.Printf("  Max Apps: %d\n", team.Spec.Quotas.MaxApps)
	fmt.Printf("  Max Resources: %d\n", team.Spec.Quotas.MaxResources)
	fmt.Printf("  Allowed Resource Types: %v\n", team.Spec.Quotas.AllowedResourceTypes)

	return printTeamDeploymentsSummary(name, store)
}

func printTeamDeploymentsSummary(team string, store *state.Store) error {
	deployments, err := store.Deployments.List(team)
	if err != nil {
		return fmt.Errorf("listing deployments for team %q: %w", team, err)
	}
	fmt.Println("Deployments:")
	if len(deployments) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	fmt.Printf("  %-30s %-15s\n", "NAME", "KIND")
	fmt.Printf("  %s\n", strings.Repeat("-", 46))
	for _, d := range deployments {
		fmt.Printf("  %-30s %-15s\n", d.Name, d.Kind)
	}
	return nil
}

// DeleteTeam removes a team from state.
func DeleteTeam(name string, store *state.Store) error {
	// 1. Check for active deployments
	deployments, err := store.Deployments.List(name)
	if err != nil {
		return fmt.Errorf("checking team deployments: %w", err)
	}
	if len(deployments) > 0 {
		return fmt.Errorf("cannot delete team %q: it has %d active deployments (run teardown first)", name, len(deployments))
	}

	// 2. Release subnet
	if err := store.Subnets.ReleaseSubnet(name); err != nil {
		return fmt.Errorf("releasing team subnet: %w", err)
	}

	// 3. Delete team from registry
	if err := store.Teams.DeleteTeam(name); err != nil {
		return err
	}

	fmt.Printf("Deleted team %q from state.\n", name)
	return nil
}
