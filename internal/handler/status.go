package handler

import (
	"fmt"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

type containerStatusRow struct {
	Name    string
	Kind    string
	Running bool
	Status  string
	ImageID string
}

func inspectDeployments(deployments []teamedDeployment, backend engine.ContainerBackend) ([]containerStatusRow, error) {
	rows := make([]containerStatusRow, 0, len(deployments))
	for _, td := range deployments {
		info, err := backend.InspectContainer(td.Deployment.ContainerID)
		if err != nil {
			return nil, err
		}
		imagePreview := info.ImageID
		if len(imagePreview) > 19 {
			imagePreview = imagePreview[:19]
		}
		rows = append(rows, containerStatusRow{
			Name:    td.Deployment.Name,
			Kind:    td.Deployment.Kind,
			Running: info.Running,
			Status:  info.Status,
			ImageID: imagePreview,
		})
	}
	return rows, nil
}

func printStatusTable(rows []containerStatusRow) {
	fmt.Printf("%-25s %-15s %-10s %-12s %-19s\n", "NAME", "KIND", "RUNNING", "STATUS", "IMAGE ID")
	fmt.Println(strings.Repeat("-", 84))
	for _, r := range rows {
		fmt.Printf("%-25s %-15s %-10v %-12s %-19s\n",
			r.Name, r.Kind, r.Running, r.Status, r.ImageID)
	}
}

func StatusTeam(name string, store *state.Store, backend engine.ContainerBackend) error {
	deployments, err := collectTeamDeployments(name, store)
	if err != nil {
		return err
	}
	if len(deployments) == 0 {
		fmt.Printf("No deployments found for team %q.\n", name)
		return nil
	}
	rows, err := inspectDeployments(deployments, backend)
	if err != nil {
		return err
	}
	printStatusTable(rows)
	return nil
}

func StatusApplication(team, name string, store *state.Store, backend engine.ContainerBackend) error {
	if team == "" {
		return statusSingleDeploymentAutoTeam(name, manifest.ApplicationKind, store, backend)
	}
	return statusSingleDeployment(team, name, manifest.ApplicationKind, store, backend)
}

func StatusResource(team, name string, store *state.Store, backend engine.ContainerBackend) error {
	if team == "" {
		return statusSingleDeploymentAutoTeam(name, manifest.ResourceKind, store, backend)
	}
	return statusSingleDeployment(team, name, manifest.ResourceKind, store, backend)
}

func statusSingleDeployment(team, name, kind string, store *state.Store, backend engine.ContainerBackend) error {
	deployments, err := store.Deployments.List(team)
	if err != nil {
		return fmt.Errorf("listing deployments for team %q: %w", team, err)
	}
	for _, d := range deployments {
		if d.Name == name && d.Kind == kind {
			rows, err := inspectDeployments([]teamedDeployment{{Team: team, Deployment: d}}, backend)
			if err != nil {
				return err
			}
			printStatusTable(rows)
			return nil
		}
	}
	return fmt.Errorf("%s %q not found in team %q", kind, name, team)
}

func statusSingleDeploymentAutoTeam(name, kind string, store *state.Store, backend engine.ContainerBackend) error {
	teams, err := store.Teams.ListTeams()
	if err != nil {
		return fmt.Errorf("listing teams: %w", err)
	}

	var matchingTeams []string
	for _, t := range teams {
		deployments, err := store.Deployments.List(t.Metadata.Name)
		if err != nil {
			continue
		}
		for _, d := range deployments {
			if d.Name == name && d.Kind == kind {
				matchingTeams = append(matchingTeams, t.Metadata.Name)
				break
			}
		}
	}

	if len(matchingTeams) == 0 {
		return fmt.Errorf("%s %q not found in any team", kind, name)
	}
	if len(matchingTeams) > 1 {
		return fmt.Errorf("ambiguous: %s %q found in teams [%s], use --team to disambiguate", kind, name, strings.Join(matchingTeams, ", "))
	}

	return statusSingleDeployment(matchingTeams[0], name, kind, store, backend)
}
