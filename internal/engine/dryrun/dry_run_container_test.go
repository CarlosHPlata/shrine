package dryrun

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/state"
)

func createOutput(t *testing.T, backend *DryRunContainerBackend, op engine.CreateContainerOp) string {
	t.Helper()
	var sb strings.Builder
	backend.Out = &sb
	if err := backend.CreateContainer(op); err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}
	return sb.String()
}

func publishOp(publish *engine.PublishPort) engine.CreateContainerOp {
	return engine.CreateContainerOp{
		Team:    "demo",
		Name:    "web",
		Image:   "nginx:alpine",
		Publish: publish,
	}
}

func TestDryRunContainer_PrintsExplicitPublishLine(t *testing.T) {
	backend := NewDryRunContainerBackend(nil)
	out := createOutput(t, backend, publishOp(&engine.PublishPort{HostPort: 8080, ContainerPort: 80}))

	if !strings.Contains(out, "publish=127.0.0.1:8080->80/tcp") {
		t.Errorf("expected explicit publish line, got:\n%s", out)
	}
}

func TestDryRunContainer_AutomaticWithoutSnapshotPrintsAuto(t *testing.T) {
	backend := NewDryRunContainerBackend(nil)
	out := createOutput(t, backend, publishOp(&engine.PublishPort{HostPort: 0, ContainerPort: 80}))

	if !strings.Contains(out, "publish=127.0.0.1:(auto)->80/tcp") {
		t.Errorf("expected (auto) placeholder, got:\n%s", out)
	}
}

func TestDryRunContainer_AutomaticWithSnapshotPrintsHeldPort(t *testing.T) {
	backend := NewDryRunContainerBackend(nil)
	backend.HostPorts = state.HostPortMap{"demo/web": 30000}
	out := createOutput(t, backend, publishOp(&engine.PublishPort{HostPort: 0, ContainerPort: 80}))

	if !strings.Contains(out, "publish=127.0.0.1:30000->80/tcp") {
		t.Errorf("expected the held port from the snapshot, got:\n%s", out)
	}
}

func TestDryRunContainer_NoPublishLineWithoutPublish(t *testing.T) {
	backend := NewDryRunContainerBackend(nil)
	out := createOutput(t, backend, publishOp(nil))

	if strings.Contains(out, "publish=") {
		t.Errorf("expected no publish line, got:\n%s", out)
	}
}

func TestDryRunContainer_ResolveImagePrintsManifestOwnedLine(t *testing.T) {
	var sb strings.Builder
	backend := NewDryRunContainerBackend(&sb)

	resolved, err := backend.ResolveImage(engine.ResolveImageOp{
		Team:            "demo",
		Name:            "web",
		Kind:            "Application",
		Image:           "reg:lab/web:1.2",
		ImagePullPolicy: "IfNotPresent",
	})
	if err != nil {
		t.Fatalf("ResolveImage failed: %v", err)
	}

	want := "[DOCKER] ImageResolve: name=demo.web image=reg:lab/web:1.2 policy=IfNotPresent -> manifest-owned\n"
	if got := sb.String(); got != want {
		t.Errorf("dry-run line:\ngot  %q\nwant %q", got, want)
	}
	wantResolved := engine.ResolvedImage{Ref: "reg:lab/web:1.2", Source: engine.ImageSourceManifest}
	if resolved != wantResolved {
		t.Errorf("resolved = %+v, want %+v (the manifest form, no image id, no digest)", resolved, wantResolved)
	}
}

func pinnedDryRunOp(image string) engine.ResolveImageOp {
	return engine.ResolveImageOp{Team: "demo", Name: "web", Kind: "Application", Image: image, ImagePullPolicy: "Pinned"}
}

func TestDryRunContainer_ResolveImagePinnedWithoutAPinSaysWouldPin(t *testing.T) {
	var sb strings.Builder
	backend := NewDryRunContainerBackend(&sb)

	resolved, err := backend.ResolveImage(pinnedDryRunOp("ghcr.io/me/web"))
	if err != nil {
		t.Fatalf("ResolveImage failed: %v", err)
	}

	want := "[DOCKER] ImageResolve: name=demo.web image=ghcr.io/me/web policy=Pinned -> would resolve newest and pin\n"
	if got := sb.String(); got != want {
		t.Errorf("dry-run line:\ngot  %q\nwant %q", got, want)
	}
	if resolved.Ref != "ghcr.io/me/web" || resolved.ImageID != "" {
		t.Errorf("resolved = %+v, want the manifest form with no image id", resolved)
	}
}

func TestDryRunContainer_ResolveImagePinnedWithAPinShowsIt(t *testing.T) {
	var sb strings.Builder
	backend := NewDryRunContainerBackend(&sb)
	pinned := "ghcr.io/me/web@sha256:3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a"
	backend.Pins = map[string]state.ImagePin{
		"demo/web": {Kind: "Application", Name: "web", Requested: "ghcr.io/me/web:latest", Pinned: pinned,
			PinnedAt: time.Date(2026, 10, 6, 10, 42, 17, 0, time.UTC)},
	}

	resolved, err := backend.ResolveImage(pinnedDryRunOp("ghcr.io/me/web"))
	if err != nil {
		t.Fatalf("ResolveImage failed: %v", err)
	}

	want := "[DOCKER] ImageResolve: name=demo.web image=ghcr.io/me/web policy=Pinned -> pinned " + pinned + " (latest, 2026-10-06)\n"
	if got := sb.String(); got != want {
		t.Errorf("dry-run line:\ngot  %q\nwant %q", got, want)
	}
	if resolved.Ref != "ghcr.io/me/web" || resolved.ImageID != "" {
		t.Errorf("resolved = %+v, want the manifest form with no image id", resolved)
	}
}

func TestNewDryRunEngine_CarriesThePinSnapshot(t *testing.T) {
	pins := map[string]state.ImagePin{"demo/web": {Name: "web"}}

	eng := NewDryRunEngine(io.Discard, nil, pins)

	backend, ok := eng.Container.(*DryRunContainerBackend)
	if !ok {
		t.Fatalf("Container = %T, want *DryRunContainerBackend", eng.Container)
	}
	if _, found := backend.Pins["demo/web"]; !found {
		t.Error("the dry-run container backend must carry the pin snapshot")
	}
}
