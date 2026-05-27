package handler

import (
	"fmt"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/planner"
)

// formatDeployPlan renders the dry-run plan summary. Inferred dependencies
// (matched against edges via (Consumer.Name, Target.Kind, Target.Name)) are
// tagged with ` (inferred from env <NAME>)`. Explicit deps carry no tag.
func formatDeployPlan(steps []planner.PlannedStep, set *planner.ManifestSet, edges []planner.InferredEdge) string {
	if len(steps) == 0 {
		return ""
	}

	envIndex := make(map[string]string, len(edges))
	for _, e := range edges {
		envIndex[edgeKey(e.Consumer.Name, e.Target.Kind, e.Target.Name)] = e.EnvVar
	}

	var b strings.Builder
	b.WriteString("Deploy order:\n")
	for i, step := range steps {
		fmt.Fprintf(&b, "  %d. %s:%s\n", i+1, step.Kind, step.Name)
		deps := stepDependencies(set, step)
		if len(deps) == 0 {
			continue
		}
		b.WriteString("       depends on:\n")
		for _, d := range deps {
			line := fmt.Sprintf("         - %s:%s", d.Kind, d.Name)
			if env, ok := envIndex[edgeKey(step.Name, d.Kind, d.Name)]; ok {
				line += fmt.Sprintf(" (inferred from env %s)", env)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func edgeKey(consumer, kind, name string) string {
	return consumer + "|" + kind + "|" + name
}

func stepDependencies(set *planner.ManifestSet, step planner.PlannedStep) []manifest.Dependency {
	switch step.Kind {
	case manifest.ApplicationKind:
		if app, ok := set.Applications[step.Name]; ok {
			return app.Spec.Dependencies
		}
	case manifest.ResourceKind:
		if res, ok := set.Resources[step.Name]; ok {
			return res.Spec.Dependencies
		}
	}
	return nil
}
