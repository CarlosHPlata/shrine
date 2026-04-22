package planner

import (
	"reflect"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

func TestOrder(t *testing.T) {
	set := &ManifestSet{
		Applications: map[string]*manifest.ApplicationManifest{
			"app-z": {Metadata: manifest.Metadata{Name: "app-z"}},
			"app-a": {Metadata: manifest.Metadata{Name: "app-a"}},
		},
		Resources: map[string]*manifest.ResourceManifest{
			"res-y": {Metadata: manifest.Metadata{Name: "res-y"}},
			"res-b": {Metadata: manifest.Metadata{Name: "res-b"}},
		},
	}

	expected := []PlannedStep{
		{Kind: manifest.ResourceKind, Name: "res-b"},
		{Kind: manifest.ResourceKind, Name: "res-y"},
		{Kind: manifest.ApplicationKind, Name: "app-a"},
		{Kind: manifest.ApplicationKind, Name: "app-z"},
	}

	actual := Order(set)

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Order() = %v, want %v", actual, expected)
	}
}
