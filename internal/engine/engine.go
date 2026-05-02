package engine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/manifest"
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

	// 1. Pre-resolve every resource up-front so applications can reference their
	// outputs via valueFrom regardless of deploy order.
	deps := resolver.ResolvedDependencies{
		Resources:    make(map[string]map[string]string, len(set.Resources)),
		Applications: make(map[string]map[string]string, len(set.Applications)),
	}

	if err := engine.Container.CreatePlatformNetwork(); err != nil {
		return engine.emitErr("network.create", map[string]string{"name": "platform"},
			fmt.Errorf("creating platform network: %w", err))
	}

	for name, res := range set.Resources {
		values, err := engine.Resolver.ResolveResource(res)
		if err != nil {
			return engine.emitErr("resource.resolve", map[string]string{"name": name},
				fmt.Errorf("resolving resource %q: %w", name, err))
		}
		deps.Resources[name] = values
	}

	// Synthesize app built-ins up-front so apps can reference each other.
	for name, app := range set.Applications {
		deps.Applications[name] = map[string]string{
			"host": app.Metadata.Owner + "." + app.Metadata.Name,
			"port": strconv.Itoa(app.Spec.Port),
		}
	}

	for _, step := range steps {
		if step.Kind == manifest.ResourceKind {
			err := engine.deployResource(set, step, deps.Resources[step.Name])
			if err != nil {
				return err
			}
		}

		if step.Kind == manifest.ApplicationKind {
			err := engine.deployApplication(set, step, deps)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (engine *Engine) ExecuteTeardown(team string, steps []planner.PlannedStep) error {
	if engine.Observer == nil {
		engine.Observer = NoopObserver{}
	}

	for _, step := range steps {
		err := engine.teardownKind(step.Kind, team, step)
		if err != nil {
			return err
		}
	}

	if err := engine.Container.RemoveNetwork(team); err != nil {
		return engine.emitErr("network.remove", map[string]string{"team": team},
			fmt.Errorf("removing network for team %q: %w", team, err))
	}
	return nil
}

func (engine *Engine) deployApplication(
	set *planner.ManifestSet,
	step planner.PlannedStep,
	deps resolver.ResolvedDependencies,
) error {
	application := set.Applications[step.Name]

	engine.Observer.OnEvent(Event{
		Name:   "application.deploy",
		Status: StatusStarted,
		Fields: map[string]string{"name": step.Name, "owner": application.Metadata.Owner},
	})

	// 1. Resolve env: static values and valueFrom references.
	envMap, err := engine.Resolver.ResolveApplication(application, deps)
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
	volumes := make([]VolumeMount, len(application.Spec.Volumes))
	for i, v := range application.Spec.Volumes {
		volumes[i] = VolumeMount{
			Name:      v.Name,
			MountPath: v.MountPath,
		}
	}

	engine.Observer.OnEvent(Event{
		Name:   "container.create",
		Status: StatusInfo,
		Fields: map[string]string{"team": application.Metadata.Owner, "name": application.Metadata.Name},
	})
	op := CreateContainerOp{
		Team:             application.Metadata.Owner,
		Name:             application.Metadata.Name,
		Kind:             manifest.ApplicationKind,
		Image:            application.Spec.Image,
		Network:          application.Metadata.Owner,
		Env:              env,
		Volumes:          volumes,
		ExposeToPlatform: application.Spec.Networking.ExposeToPlatform,
		ImagePullPolicy:  manifest.EffectivePullPolicy(application.Spec.Image, application.Spec.ImagePullPolicy),
	}
	if err := engine.Container.CreateContainer(op); err != nil {
		return engine.emitErr("container.create", map[string]string{"team": application.Metadata.Owner, "name": application.Metadata.Name},
			fmt.Errorf("application %q: %w", step.Name, err))
	}

	// 4. Write Router
	if application.Spec.Routing.Domain != "" && application.Spec.Networking.ExposeToPlatform && engine.Routing != nil {
		aliasRoutes := resolveAliasRoutes(application.Spec.Routing.Aliases)

		eventFields := map[string]string{
			"domain": application.Spec.Routing.Domain,
			"port":   fmt.Sprintf("%d", application.Spec.Port),
		}
		if len(application.Spec.Routing.Aliases) > 0 {
			eventFields["aliases"] = formatAliasesForLog(aliasRoutes)
		}
		engine.Observer.OnEvent(Event{
			Name:   "routing.configure",
			Status: StatusInfo,
			Fields: eventFields,
		})
		routingOp := WriteRouteOp{
			Team:             application.Metadata.Owner,
			Domain:           application.Spec.Routing.Domain,
			ServiceName:      application.Metadata.Name,
			ServicePort:      application.Spec.Port,
			PathPrefix:       application.Spec.Routing.PathPrefix,
			AdditionalRoutes: aliasRoutes,
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

func (engine *Engine) teardownKind(kind string, team string, step planner.PlannedStep) error {
	engine.Observer.OnEvent(Event{
		Name:   kind + ".teardown",
		Status: StatusStarted,
		Fields: map[string]string{"team": team, "name": step.Name},
	})

	op := RemoveContainerOp{
		Team: team,
		Name: step.Name,
	}
	if err := engine.Container.RemoveContainer(op); err != nil {
		return engine.emitErr(kind+".remove", map[string]string{"team": team, "name": step.Name},
			fmt.Errorf("%s %q: %w", kind, step.Name, err))
	}

	if step.Kind == manifest.ApplicationKind && engine.Routing != nil {
		if err := engine.Routing.RemoveRoute(team, step.Name); err != nil {
			return engine.emitErr(kind+".routing_remove", map[string]string{"team": team, "name": step.Name},
				fmt.Errorf("%s %q routing: %w", kind, step.Name, err))
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
	volumes := make([]VolumeMount, len(resource.Spec.Volumes))
	for i, v := range resource.Spec.Volumes {
		volumes[i] = VolumeMount{
			Name:      v.Name,
			MountPath: v.MountPath,
		}
	}

	engine.Observer.OnEvent(Event{
		Name:   "container.create",
		Status: StatusInfo,
		Fields: map[string]string{"team": resource.Metadata.Owner, "name": resource.Metadata.Name},
	})
	op := CreateContainerOp{
		Team:             resource.Metadata.Owner,
		Name:             resource.Metadata.Name,
		Kind:             manifest.ResourceKind,
		Image:            resource.Spec.Image,
		Network:          resource.Metadata.Owner,
		Env:              env,
		Volumes:          volumes,
		ExposeToPlatform: resource.Spec.Networking.ExposeToPlatform,
		ImagePullPolicy:  manifest.EffectivePullPolicy(resource.Spec.Image, resource.Spec.ImagePullPolicy),
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

func resolveAliasRoutes(aliases []manifest.RoutingAlias) []AliasRoute {
	routes := make([]AliasRoute, 0, len(aliases))
	for _, alias := range aliases {
		prefix := strings.TrimRight(alias.PathPrefix, "/")
		var strip bool
		if alias.StripPrefix != nil {
			strip = *alias.StripPrefix
		} else {
			strip = prefix != ""
		}
		routes = append(routes, AliasRoute{
			Host:        alias.Host,
			PathPrefix:  prefix,
			StripPrefix: strip,
			TLS:         alias.TLS,
		})
	}
	return routes
}

func formatAliasesForLog(routes []AliasRoute) string {
	entries := make([]string, 0, len(routes))
	for _, r := range routes {
		entry := r.Host
		if r.PathPrefix != "" {
			entry += "+" + r.PathPrefix
		}
		if r.PathPrefix != "" && !r.StripPrefix {
			entry += " (no strip)"
		}
		if r.TLS {
			entry += " (tls)"
		}
		entries = append(entries, entry)
	}
	sort.Strings(entries)
	return strings.Join(entries, ",")
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
