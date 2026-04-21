package engine

import (
	"fmt"
	"sort"

	"github.com/CarlosHPlata/shrine/internal/planner"
	"github.com/CarlosHPlata/shrine/internal/resolver"
)

type Engine struct {
	Container ContainerBackend
	Routing   RoutingBackend
	DNS       DNSBackend
	Resolver  resolver.Resolver
	Observer  Observer
}

func (engine *Engine) emitErr(name string, fields map[string]string, err error) error {
	if fields == nil {
		fields = map[string]string{}
	}
	fields["error"] = err.Error()
	engine.Observer.OnEvent(Event{Name: name, Status: StatusError, Fields: fields})
	return err
}

func (engine *Engine) ExecuteDeploy(steps []planner.PlannedStep, set *planner.ManifestSet) error {
	if engine.Observer == nil {
		engine.Observer = NoopObserver{}
	}

	// Pre-resolve every resource up-front so applications can reference their
	// outputs via valueFrom regardless of deploy order.
	resolvedResources := make(map[string]map[string]string, len(set.Resources))
	for name, res := range set.Resources {
		values, err := engine.Resolver.ResolveResource(res)
		if err != nil {
			return engine.emitErr("resource.resolve", map[string]string{"name": name},
				fmt.Errorf("resolving resource %q: %w", name, err))
		}
		resolvedResources[name] = values
	}

	for _, step := range steps {
		if step.Kind == "Resource" {
			err := engine.deployResource(set, step, resolvedResources[step.Name])
			if err != nil {
				return err
			}
		}

		if step.Kind == "Application" {
			err := engine.deployApplication(set, step, resolvedResources)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (engine *Engine) deployApplication(set *planner.ManifestSet, step planner.PlannedStep, resolvedResources map[string]map[string]string) error {
	application := set.Applications[step.Name]

	engine.Observer.OnEvent(Event{
		Name:   "application.deploy",
		Status: StatusStarted,
		Fields: map[string]string{"name": step.Name, "owner": application.Metadata.Owner},
	})

	// 1. Resolve env: static values and valueFrom references.
	envMap, err := engine.Resolver.ResolveApplication(application, resolvedResources)
	if err != nil {
		return engine.emitErr("application.resolve", map[string]string{"name": step.Name},
			fmt.Errorf("application %q: %w", step.Name, err))
	}
	env := flattenEnv(envMap)

	// 2. Orchestrate network creation IF IT DOESN'T EXIST
	engine.Observer.OnEvent(Event{
		Name:   "network.ensure",
		Status: StatusInfo,
		Fields: map[string]string{"owner": application.Metadata.Owner},
	})
	if err := engine.Container.CreateNetwork(application.Metadata.Owner); err != nil {
		return engine.emitErr("network.ensure", map[string]string{"owner": application.Metadata.Owner, "name": step.Name},
			fmt.Errorf("application %q: %w", step.Name, err))
	}

	// 3. Create the container
	engine.Observer.OnEvent(Event{
		Name:   "container.create",
		Status: StatusInfo,
		Fields: map[string]string{"team": application.Metadata.Owner, "name": application.Metadata.Name},
	})
	op := CreateContainerOp{
		Team:    application.Metadata.Owner,
		Name:    application.Metadata.Name,
		Kind:    "Application",
		Image:   application.Spec.Image,
		Network: application.Metadata.Owner,
		Env:     env,
	}
	if err := engine.Container.CreateContainer(op); err != nil {
		return engine.emitErr("container.create", map[string]string{"team": application.Metadata.Owner, "name": application.Metadata.Name},
			fmt.Errorf("application %q: %w", step.Name, err))
	}

	// 4. Write Router
	if application.Spec.Routing.Domain != "" && engine.Routing != nil {
		engine.Observer.OnEvent(Event{
			Name:   "routing.configure",
			Status: StatusInfo,
			Fields: map[string]string{
				"domain": application.Spec.Routing.Domain,
				"port":   fmt.Sprintf("%d", application.Spec.Port),
			},
		})
		routingOp := WriteRouteOp{
			Team:        application.Metadata.Owner,
			Domain:      application.Spec.Routing.Domain,
			ServiceName: application.Metadata.Name,
			ServicePort: application.Spec.Port,
			PathPrefix:  application.Spec.Routing.PathPrefix,
		}

		if err := engine.Routing.WriteRoute(routingOp); err != nil {
			return engine.emitErr("routing.configure", map[string]string{"domain": application.Spec.Routing.Domain, "name": step.Name},
				fmt.Errorf("application %q: %w", step.Name, err))
		}
	}

	// 5. Write DNS
	if application.Spec.Routing.Domain != "" && engine.DNS != nil {
		engine.Observer.OnEvent(Event{
			Name:   "dns.register",
			Status: StatusInfo,
			Fields: map[string]string{"domain": application.Spec.Routing.Domain},
		})
		dnsOp := WriteRecordOp{
			Team:       application.Metadata.Owner,
			Name:       application.Spec.Routing.Domain,
			RecordType: "A",
			Value:      "[IP_ADDRESS]",
		}
		if err := engine.DNS.WriteRecord(dnsOp); err != nil {
			return engine.emitErr("dns.register", map[string]string{"domain": application.Spec.Routing.Domain, "name": step.Name},
				fmt.Errorf("application %q: %w", step.Name, err))
		}
	}
	return nil
}

func (engine *Engine) deployResource(set *planner.ManifestSet, step planner.PlannedStep, resolvedValues map[string]string) error {
	resource := set.Resources[step.Name]

	engine.Observer.OnEvent(Event{
		Name:   "resource.deploy",
		Status: StatusStarted,
		Fields: map[string]string{"name": step.Name, "type": resource.Spec.Type},
	})

	// 1. Flatten the pre-resolved outputs into env. Built-ins (team/name) are
	// dropped since they aren't meaningful as container env vars.
	env := flattenOutputs(resolvedValues)

	// 2. Orchestrate network creation
	engine.Observer.OnEvent(Event{
		Name:   "network.ensure",
		Status: StatusInfo,
		Fields: map[string]string{"owner": resource.Metadata.Owner},
	})
	if err := engine.Container.CreateNetwork(resource.Metadata.Owner); err != nil {
		return engine.emitErr("network.ensure", map[string]string{"owner": resource.Metadata.Owner, "name": step.Name},
			fmt.Errorf("resource %q: %w", step.Name, err))
	}

	// 3. Create the container
	engine.Observer.OnEvent(Event{
		Name:   "container.create",
		Status: StatusInfo,
		Fields: map[string]string{"team": resource.Metadata.Owner, "name": resource.Metadata.Name},
	})
	op := CreateContainerOp{
		Team:    resource.Metadata.Owner,
		Name:    resource.Metadata.Name,
		Kind:    "Resource",
		Image:   resource.Spec.Image,
		Network: resource.Metadata.Owner,
		Env:     env,
	}
	if err := engine.Container.CreateContainer(op); err != nil {
		return engine.emitErr("container.create", map[string]string{"team": resource.Metadata.Owner, "name": resource.Metadata.Name},
			fmt.Errorf("resource %q: %w", step.Name, err))
	}
	return nil
}

// flattenEnv converts a map to a sorted KEY=VALUE slice for deterministic output.
func flattenEnv(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(env))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

// flattenOutputs is like flattenEnv but skips built-in keys that aren't part
// of the resource's declared outputs.
func flattenOutputs(values map[string]string) []string {
	filtered := make(map[string]string, len(values))
	for k, v := range values {
		if k == "team" || k == "name" {
			continue
		}
		filtered[k] = v
	}
	return flattenEnv(filtered)
}
