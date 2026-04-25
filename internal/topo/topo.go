package topo

import (
	"fmt"
	"sort"
)

// Sort returns the nodes in an order where each appears after its
// dependencies, using Kahn's algorithm. Cycles produce an error listing the
// unresolved nodes.
func Sort(deps map[string]map[string]struct{}) ([]string, error) {
	// reverse[x] = set of nodes that depend on x.
	reverse := make(map[string]map[string]struct{}, len(deps))
	indeg := make(map[string]int, len(deps))
	for node := range deps {
		indeg[node] = 0
		reverse[node] = make(map[string]struct{})
	}
	for node, ds := range deps {
		for d := range ds {
			if _, ok := deps[d]; !ok {
				// dependency isn't in the graph — already resolved, skip.
				continue
			}
			reverse[d][node] = struct{}{}
			indeg[node]++
		}
	}

	var queue []string
	for node, deg := range indeg {
		if deg == 0 {
			queue = append(queue, node)
		}
	}
	sort.Strings(queue)

	var order []string
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)

		var newlyFreed []string
		for dep := range reverse[n] {
			indeg[dep]--
			if indeg[dep] == 0 {
				newlyFreed = append(newlyFreed, dep)
			}
		}
		sort.Strings(newlyFreed)
		queue = append(queue, newlyFreed...)
	}

	if len(order) != len(deps) {
		var stuck []string
		for node, deg := range indeg {
			if deg > 0 {
				stuck = append(stuck, node)
			}
		}
		sort.Strings(stuck)
		return nil, fmt.Errorf("dependency cycle involving: %v", stuck)
	}
	return order, nil
}
