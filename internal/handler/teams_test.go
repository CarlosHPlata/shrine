package handler

import (
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

func TestDeleteTeam_ReleasesEveryPinOfTheTeam(t *testing.T) {
	store := deleteTestStore([]string{"demo", "media"}, state.HostPortMap{}, nil)
	pins := newMemImagePinStore(
		state.ImagePin{Kind: manifest.ApplicationKind, Name: "api", Pinned: "ghcr.io/me/api@sha256:abc"},
		state.ImagePin{Kind: manifest.ResourceKind, Name: "db", Pinned: "postgres@sha256:def"},
	)
	pins.pins[state.ImagePinKey("media", "web")] = state.ImagePin{Kind: manifest.ApplicationKind, Name: "web", Pinned: "nginx@sha256:fff"}
	store.ImagePins = pins

	if err := DeleteTeam("demo", store); err != nil {
		t.Fatalf("DeleteTeam failed: %v", err)
	}

	all, _ := store.ImagePins.ListAll()
	if len(all) != 1 {
		t.Fatalf("expected only the other team's pin to remain, got %v", all)
	}
	if _, ok := all["media/web"]; !ok {
		t.Errorf("the other team's pin must be untouched, got %v", all)
	}
}

func TestDeleteTeam_ToleratesAStoreWithoutPins(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, state.HostPortMap{}, nil)

	if err := DeleteTeam("demo", store); err != nil {
		t.Fatalf("DeleteTeam must tolerate a nil ImagePins store, got %v", err)
	}
}
