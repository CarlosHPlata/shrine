package handler

import (
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/app"
	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine/dryrun"
	"github.com/CarlosHPlata/shrine/internal/planner"
	"github.com/CarlosHPlata/shrine/internal/plugins/gateway/traefik"
	"github.com/CarlosHPlata/shrine/internal/state"
)

// imagePinSnapshot reads every recorded pin once for the preview; a store
// without pins yields an empty snapshot.
func imagePinSnapshot(store *state.Store) (map[string]state.ImagePin, error) {
	if store == nil || store.ImagePins == nil {
		return map[string]state.ImagePin{}, nil
	}
	pins, err := store.ImagePins.ListAll()
	if err != nil {
		return nil, fmt.Errorf("listing image pins: %w", err)
	}
	return pins, nil
}

// buildPortContext gathers the host-port knowledge living outside the
// manifest set: gateway-reserved ports from config and persisted allocations
// from state.
func buildPortContext(store *state.Store, cfg *config.Config) (planner.PortContext, error) {
	ports := planner.PortContext{}
	if cfg != nil {
		ports.Reserved = traefik.ReservedHostPorts(cfg.Plugins.Gateway.Traefik)
	}
	if store != nil && store.HostPorts != nil {
		persisted, err := store.HostPorts.ListHostPorts()
		if err != nil {
			return planner.PortContext{}, fmt.Errorf("listing host port allocations: %w", err)
		}
		ports.Persisted = persisted
	}
	return ports, nil
}

// planManifestSet plans a loaded set the way every manifest-driven command
// does, so bump never accepts a set that deploy would reject. The port
// context is returned so a caller needing it again does not read state twice.
func planManifestSet(errOut io.Writer, set *planner.ManifestSet, store *state.Store, cfg *config.Config, filter planner.Filter) (planner.PlanResult, planner.PortContext, error) {
	ports, err := buildPortContext(store, cfg)
	if err != nil {
		return planner.PlanResult{}, planner.PortContext{}, err
	}
	result := planner.Plan(set, store.Teams, cfg.Registries, ports, filter, cfg.ImagePullPolicy)

	if result.Error != nil {
		return planner.PlanResult{}, planner.PortContext{}, result.Error
	}

	if len(result.ValidationErr) > 0 {
		fmt.Fprintln(errOut, "Validation errors:")
		for _, err := range result.ValidationErr {
			fmt.Fprintln(errOut, err)
		}
		return planner.PlanResult{}, planner.PortContext{}, fmt.Errorf("Spec validation errors")
	}
	return result, ports, nil
}

// DryRun runs a dry-run deploy scoped by filter. When cfg is non-nil, registries
// and the Traefik config are validated; the dry-run engine prints route
// operations instead of writing files. Planning output goes to out, validation
// errors to errOut.
func DryRun(out, errOut io.Writer, manifestDir string, store *state.Store, cfg *config.Config, filter planner.Filter) error {
	if cfg != nil {
		if err := cfg.ValidateRegistries(); err != nil {
			return err
		}
		if err := app.ValidateTraefikConfig(cfg); err != nil {
			return err
		}
	}

	set, err := planner.LoadDir(manifestDir)
	if err != nil {
		return err
	}
	result, ports, err := planManifestSet(errOut, set, store, cfg, filter)
	if err != nil {
		return err
	}

	if len(result.Steps) == 0 {
		fmt.Fprintln(out, "No steps generated.")
		return nil
	}

	fmt.Fprint(out, formatDeployPlan(result.Steps, result.ManifestSet, result.InferredEdges))

	pins, err := imagePinSnapshot(store)
	if err != nil {
		return err
	}
	engineInst := dryrun.NewDryRunEngine(out, ports.Persisted, pins)
	if err := engineInst.ExecuteDeploy(result.Steps, result.ManifestSet); err != nil {
		return err
	}

	return nil
}

func Deploy(b *app.DeployBundle, manifestDir string, filter planner.Filter) error {
	set, err := planner.LoadDir(manifestDir)
	if err != nil {
		return err
	}
	result, _, err := planManifestSet(b.ErrOut, set, b.Store, b.Cfg, filter)
	if err != nil {
		return err
	}

	if len(result.Steps) == 0 {
		fmt.Fprintln(b.Out, "No steps generated.")
		return nil
	}

	if err := b.Engine.ExecuteDeploy(result.Steps, result.ManifestSet); err != nil {
		return err
	}

	return nil
}
