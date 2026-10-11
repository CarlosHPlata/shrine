package handler

import (
	"time"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

type pinEntry struct {
	Reference string `json:"reference"`
	Version   string `json:"version"`
	PinnedAt  string `json:"pinnedAt"`
}

type deploymentEntry struct {
	Team         string    `json:"team"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	Image        string    `json:"image"`
	PullPolicy   string    `json:"pullPolicy"`
	Version      string    `json:"version"`
	Pin          *pinEntry `json:"pin"`
	RunningImage *string   `json:"runningImage"`
	ContainerID  string    `json:"containerId"`
	ConfigHash   string    `json:"configHash"`
}

func ListDeployedJSON(team string, store *state.Store, backend engine.ContainerBackend) error {
	return listDeploymentsJSON(team, "", store, backend)
}

func ListApplicationsJSON(team string, store *state.Store, backend engine.ContainerBackend) error {
	return listDeploymentsJSON(team, manifest.ApplicationKind, store, backend)
}

func ListResourcesJSON(team string, store *state.Store, backend engine.ContainerBackend) error {
	return listDeploymentsJSON(team, manifest.ResourceKind, store, backend)
}

func listDeploymentsJSON(team, kind string, store *state.Store, backend engine.ContainerBackend) error {
	deployments, err := listDeployments(team, kind, store)
	if err != nil {
		return err
	}
	pins, err := loadImagePins(store)
	if err != nil {
		return err
	}
	doc, err := formatDeploymentsJSON(deployments, pins, backend)
	if err != nil {
		return err
	}
	return printListing(doc)
}

func formatDeploymentsJSON(deployments []teamedDeployment, pins map[string]state.ImagePin, backend engine.ContainerBackend) ([]byte, error) {
	entries := make([]deploymentEntry, len(deployments))
	for i, td := range deployments {
		entries[i] = toDeploymentEntry(td, pins, backend)
	}
	return encodeListing(entries)
}

func toDeploymentEntry(td teamedDeployment, pins map[string]state.ImagePin, backend engine.ContainerBackend) deploymentEntry {
	d := td.Deployment
	return deploymentEntry{
		Team:         td.Team,
		Name:         d.Name,
		Kind:         d.Kind,
		Image:        d.Image,
		PullPolicy:   d.Policy,
		Version:      versionOf(td.Team, d, pins),
		Pin:          pinEntryOf(td.Team, d, pins),
		RunningImage: runningImageOrNull(backend, d.ContainerID),
		ContainerID:  d.ContainerID,
		ConfigHash:   d.ConfigHash,
	}
}

func pinEntryOf(team string, d state.Deployment, pins map[string]state.ImagePin) *pinEntry {
	pin, ok := pinOf(team, d, pins)
	if !ok {
		return nil
	}
	return &pinEntry{
		Reference: pin.Pinned,
		Version:   readablePin(pin),
		PinnedAt:  pin.PinnedAt.UTC().Format(time.RFC3339),
	}
}

// runningImageOrNull keeps runtime trouble out of the listing's exit code:
// whatever the runtime cannot report is null, never an error.
func runningImageOrNull(backend engine.ContainerBackend, containerID string) *string {
	if containerID == "" {
		return nil
	}
	image, err := inspectRunningImage(backend, containerID)
	if err != nil {
		return nil
	}
	return &image
}
