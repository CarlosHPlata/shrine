package topo

import (
	"strings"
	"testing"
)

func TestSort(t *testing.T) {
	tests := []struct {
		name    string
		deps    map[string]map[string]struct{}
		wantErr string
	}{
		{
			name: "simple dag",
			deps: map[string]map[string]struct{}{
				"a": {"b": struct{}{}},
				"b": {"c": struct{}{}},
				"c": {},
			},
		},
		{
			name: "diamond",
			deps: map[string]map[string]struct{}{
				"a": {"b": struct{}{}, "c": struct{}{}},
				"b": {"d": struct{}{}},
				"c": {"d": struct{}{}},
				"d": {},
			},
		},
		{
			name: "cycle",
			deps: map[string]map[string]struct{}{
				"a": {"b": struct{}{}},
				"b": {"c": struct{}{}},
				"c": {"a": struct{}{}},
			},
			wantErr: "dependency cycle",
		},
		{
			name: "disconnected",
			deps: map[string]map[string]struct{}{
				"a": {"b": struct{}{}},
				"b": {},
				"c": {"d": struct{}{}},
				"d": {},
			},
		},
		{
			name: "missing dependency (should skip)",
			deps: map[string]map[string]struct{}{
				"a": {"external": struct{}{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Sort(tt.deps)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Sort() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Sort() unexpected error: %v", err)
			}

			// Validate topological property: for every node, its deps must appear earlier.
			pos := make(map[string]int)
			for i, node := range got {
				pos[node] = i
			}

			if len(got) != len(tt.deps) {
				t.Errorf("Sort() returned %d nodes, want %d", len(got), len(tt.deps))
			}

			for node, deps := range tt.deps {
				for d := range deps {
					// Only check dependencies that were part of the input graph
					if _, ok := tt.deps[d]; !ok {
						continue
					}
					if pos[node] < pos[d] {
						t.Errorf("node %q (pos %d) appears before its dependency %q (pos %d)",
							node, pos[node], d, pos[d])
					}
				}
			}
		})
	}
}
