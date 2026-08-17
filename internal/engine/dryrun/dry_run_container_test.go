package dryrun

import (
	"strings"
	"testing"

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
