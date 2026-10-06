package dryrun

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

// DryRunContainerBackend implements ContainerBackend by printing Docker operations.
type DryRunContainerBackend struct {
	Out      io.Writer
	Networks map[string]bool
	// HostPorts is a read-only snapshot of persisted allocations used to
	// preview which port an automatic publish already holds.
	HostPorts state.HostPortMap
	// Pins is a read-only snapshot of recorded image pins, keyed team/name,
	// used to preview whether a Pinned artifact would pin or reuse.
	Pins map[string]state.ImagePin
}

func NewDryRunContainerBackend(out io.Writer) *DryRunContainerBackend {
	return &DryRunContainerBackend{
		Out:      out,
		Networks: make(map[string]bool),
	}
}

func (d *DryRunContainerBackend) CreateNetwork(name string) error {
	if d.Networks[name] {
		return nil
	}

	d.Networks[name] = true
	fmt.Fprintf(d.Out, "[DOCKER] NetworkCreate: name=%s\n", name)
	return nil
}

func (d *DryRunContainerBackend) RemoveNetwork(name string) error {
	fmt.Fprintf(d.Out, "[DOCKER] NetworkRemove: name=%s\n", name)
	return nil
}

func (d *DryRunContainerBackend) CreateContainer(op engine.CreateContainerOp) error {
	fmt.Fprintf(d.Out, "[DOCKER] ContainerCreate: name=%s.%s image=%s", op.Team, op.Name, op.Image)

	if len(op.Volumes) > 0 {
		parts := make([]string, len(op.Volumes))
		for i, v := range op.Volumes {
			parts[i] = fmt.Sprintf("%s:%s", v.Name, v.MountPath)
		}
		fmt.Fprintf(d.Out, "\n  volumes=%s", strings.Join(parts, ", "))
	}

	if op.ExposeToPlatform {
		fmt.Fprintf(d.Out, "\n  attach to platform network=shrine.platform")
	}

	if op.Publish != nil {
		fmt.Fprintf(d.Out, "\n  publish=127.0.0.1:%s->%d/tcp", d.publishHostPortLabel(op), op.Publish.ContainerPort)
	}

	fmt.Fprintln(d.Out)
	return nil
}

// publishHostPortLabel previews the host port without allocating: explicit
// ports print as-is, an automatic port already persisted for this app prints
// as held, and a first-time automatic request shows "(auto)".
func (d *DryRunContainerBackend) publishHostPortLabel(op engine.CreateContainerOp) string {
	if op.Publish.HostPort > 0 {
		return strconv.Itoa(op.Publish.HostPort)
	}
	if held, ok := d.HostPorts[state.HostPortKey(op.Team, op.Name)]; ok {
		return strconv.Itoa(held)
	}
	return "(auto)"
}

func (d *DryRunContainerBackend) RemoveContainer(op engine.RemoveContainerOp) error {
	fmt.Fprintf(d.Out, "[DOCKER] ContainerRemove: name=%s.%s\n", op.Team, op.Name)
	return nil
}

func (d *DryRunContainerBackend) CreatePlatformNetwork() error {
	fmt.Fprintf(d.Out, "[DOCKER] CreatePlatformNetwork name=shrine.platform\n")
	return nil
}

func (d *DryRunContainerBackend) InspectContainer(containerID string) (engine.ContainerInfo, error) {
	return engine.ContainerInfo{}, nil
}

func (d *DryRunContainerBackend) ResolveImage(op engine.ResolveImageOp) (engine.ResolvedImage, error) {
	fmt.Fprintf(d.Out, "[DOCKER] ImageResolve: name=%s.%s image=%s policy=%s -> %s\n",
		op.Team, op.Name, op.Image, op.ImagePullPolicy, d.imageDecision(op))
	return engine.ResolvedImage{Ref: op.Image, Source: engine.ImageSourceManifest}, nil
}

// imageDecision previews the pinned branches from the snapshot alone: the
// preview expands no alias, so it shows the pin on record as is.
func (d *DryRunContainerBackend) imageDecision(op engine.ResolveImageOp) string {
	if op.ImagePullPolicy != manifest.ImagePullPolicyPinned {
		return "manifest-owned"
	}
	pin, ok := d.Pins[state.ImagePinKey(op.Team, op.Name)]
	if !ok {
		return "would resolve newest and pin"
	}
	return fmt.Sprintf("pinned %s (%s, %s)", pin.Pinned, requestedTag(pin.Requested), pin.PinnedAt.UTC().Format(time.DateOnly))
}

func requestedTag(requested string) string {
	if manifest.IsDigestReference(requested) {
		_, digest, _ := strings.Cut(requested, "@")
		return digest
	}
	if tag := manifest.TagOf(requested); tag != "" {
		return tag
	}
	return "latest"
}
