package engine

import (
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/engine/backends"
	"github.com/CarlosHPlata/shrine/internal/planner"
)

type Engine struct {
	Container backends.ContainerBackend
	Routing   backends.RoutingBackend
	DNS       backends.DNSBackend
}

func NewDryRunEngine(out io.Writer) *Engine {
	return &Engine{
		Container: backends.NewDryRunContainerBackend(out),
		Routing:   &backends.DryRunRoutingBackend{Out: out},
		DNS:       &backends.DryRunDNSBackend{Out: out},
	}
}

func (engine *Engine) ExecuteDeploy(steps []planner.PlannedStep, set *planner.ManifestSet) error {
	for _, step := range steps {
		if step.Kind == "Resource" {
			err := engine.deployResource(set, step)
			if err != nil {
				return err
			}
		}

		if step.Kind == "Application" {
			err := engine.deployApplication(set, step)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (engine *Engine) deployApplication(set *planner.ManifestSet, step planner.PlannedStep) error {
	application := set.Applications[step.Name]

	// 1. Get our pre-computed environment
	env := application.Spec.StaticEnv()

	// 2. Orchestrate network creation IF IT DOESN'T EXIST
	if err := engine.Container.CreateNetwork(application.Metadata.Owner); err != nil {
		return fmt.Errorf("application %q: %w", step.Name, err)
	}

	// 3. Create the container
	op := backends.CreateContainerOp{
		Name:    application.Metadata.Name,
		Image:   application.Spec.Image,
		Network: application.Metadata.Owner,
		Env:     env,
	}
	if err := engine.Container.CreateContainer(op); err != nil {
		return fmt.Errorf("application %q: %w", step.Name, err)
	}

	// 4. Write Router
	if application.Spec.Routing.Domain != "" && engine.Routing != nil {
		routingOp := backends.WriteRouteOp{
			Team:        application.Metadata.Owner,
			Domain:      application.Spec.Routing.Domain,
			ServiceName: application.Metadata.Name,
			ServicePort: application.Spec.Port,
			PathPrefix:  application.Spec.Routing.PathPrefix,
		}

		if err := engine.Routing.WriteRoute(routingOp); err != nil {
			return fmt.Errorf("application %q: %w", step.Name, err)
		}
	}

	// 5. Write DNS
	if application.Spec.Routing.Domain != "" && engine.DNS != nil {
		dnsOp := backends.WriteRecordOp{
			Team:       application.Metadata.Owner,
			Name:       application.Spec.Routing.Domain,
			RecordType: "A",
			Value:      "[IP_ADDRESS]",
		}
		if err := engine.DNS.WriteRecord(dnsOp); err != nil {
			return fmt.Errorf("application %q: %w", step.Name, err)
		}
	}
	return nil
}

func (engine *Engine) deployResource(set *planner.ManifestSet, step planner.PlannedStep) error {
	resource := set.Resources[step.Name]

	// 1. Get our pre-computed environment
	env := resource.Spec.StaticEnv()

	// 2. Orchestrate network creation
	if err := engine.Container.CreateNetwork(resource.Metadata.Owner); err != nil {
		return fmt.Errorf("resource %q: %w", step.Name, err)
	}

	// 3. Create the container
	op := backends.CreateContainerOp{
		Name:    resource.Metadata.Name,
		Image:   resource.Spec.Image,
		Network: resource.Metadata.Owner,
		Env:     env,
	}
	if err := engine.Container.CreateContainer(op); err != nil {
		return fmt.Errorf("resource %q: %w", step.Name, err)
	}
	return nil
}
