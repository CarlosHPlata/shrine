//go:build integration

package testutils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/go-connections/nat"
)

const (
	registryImage     = "registry:2"
	registryPort      = "5000/tcp"
	registryReadyWait = 30 * time.Second
	// emptyRegistryAuth is the base64 form of "{}": the SDK requires the
	// header and a loopback registry needs no credentials.
	emptyRegistryAuth = "e30="
)

// LocalRegistry is a registry:2 container bound to the loopback interface,
// the only way a suite can move a tag's "latest" and prove a pin held.
type LocalRegistry struct {
	Host        string
	containerID string
}

// StartLocalRegistry runs registry:2 on 127.0.0.1 with a random host port,
// waits until it answers, and removes it when the test ends.
func StartLocalRegistry(tc *TestCase) *LocalRegistry {
	tc.t.Helper()
	ctx := context.Background()
	cli := tc.DockerClient

	tc.ensureImagePresent(registryImage)

	resp, err := cli.ContainerCreate(ctx,
		&container.Config{Image: registryImage, ExposedPorts: nat.PortSet{registryPort: struct{}{}}},
		&container.HostConfig{PortBindings: nat.PortMap{registryPort: []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "0"}}}},
		nil, nil, fmt.Sprintf("shrine-test-registry-%d", time.Now().UnixNano()))
	if err != nil {
		tc.t.Fatalf("creating registry container: %v", err)
	}
	tc.t.Cleanup(func() {
		_ = cli.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true})
	})

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		tc.t.Fatalf("starting registry container: %v", err)
	}

	info, err := cli.ContainerInspect(ctx, resp.ID)
	if err != nil {
		tc.t.Fatalf("inspecting registry container: %v", err)
	}
	bindings := info.NetworkSettings.Ports[registryPort]
	if len(bindings) == 0 {
		tc.t.Fatalf("registry container exposes no host port for %s", registryPort)
	}
	registry := &LocalRegistry{Host: "127.0.0.1:" + bindings[0].HostPort, containerID: resp.ID}
	registry.waitUntilReady(tc)
	return registry
}

func (r *LocalRegistry) waitUntilReady(tc *TestCase) {
	tc.t.Helper()
	deadline := time.Now().Add(registryReadyWait)
	url := "http://" + r.Host + "/v2/"
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	tc.t.Fatalf("registry at %s did not answer within %s", r.Host, registryReadyWait)
}

// PushAs tags source as <host>/<repoTag>, pushes it, and returns the digest
// the registry assigned, as the daemon records it after the push.
func (r *LocalRegistry) PushAs(tc *TestCase, source, repoTag string) string {
	tc.t.Helper()
	ctx := context.Background()
	cli := tc.DockerClient
	target := r.Host + "/" + repoTag

	tc.ensureImagePresent(source)
	if err := cli.ImageTag(ctx, source, target); err != nil {
		tc.t.Fatalf("tagging %s as %s: %v", source, target, err)
	}

	reader, err := cli.ImagePush(ctx, target, image.PushOptions{RegistryAuth: emptyRegistryAuth})
	if err != nil {
		tc.t.Fatalf("pushing %s: %v", target, err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		tc.t.Fatalf("reading push stream for %s: %v", target, err)
	}
	if strings.Contains(string(body), `"error"`) {
		tc.t.Fatalf("push of %s reported an error:\n%s", target, body)
	}

	inspected, err := cli.ImageInspect(ctx, target)
	if err != nil {
		tc.t.Fatalf("inspecting %s after push: %v", target, err)
	}
	prefix := r.Host + "/" + repositoryOfTag(repoTag) + "@"
	for _, entry := range inspected.RepoDigests {
		if digest, found := strings.CutPrefix(entry, prefix); found {
			return digest
		}
	}
	tc.t.Fatalf("no repository digest for %s after push; RepoDigests = %v", target, inspected.RepoDigests)
	return ""
}

func repositoryOfTag(repoTag string) string {
	if colon := strings.LastIndex(repoTag, ":"); colon > strings.LastIndex(repoTag, "/") {
		return repoTag[:colon]
	}
	return repoTag
}

// ImageIDOf returns the local image id of ref, failing the test when absent.
func (tc *TestCase) ImageIDOf(ref string) string {
	tc.t.Helper()
	inspected, err := tc.DockerClient.ImageInspect(context.Background(), ref)
	if err != nil {
		tc.t.Fatalf("inspecting image %s: %v", ref, err)
	}
	return inspected.ID
}

// RemoveImage force-removes ref from the host; the "wiped cache" cycle.
func (tc *TestCase) RemoveImage(ref string) {
	tc.t.Helper()
	_, err := tc.DockerClient.ImageRemove(context.Background(), ref, image.RemoveOptions{Force: true, PruneChildren: true})
	if err != nil {
		tc.t.Fatalf("removing image %s: %v", ref, err)
	}
}

func (tc *TestCase) ensureImagePresent(ref string) {
	tc.t.Helper()
	ctx := context.Background()
	if _, err := tc.DockerClient.ImageInspect(ctx, ref); err == nil {
		return
	} else if !errdefs.IsNotFound(err) {
		tc.t.Fatalf("inspecting image %s: %v", ref, err)
	}
	reader, err := tc.DockerClient.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		tc.t.Fatalf("pulling %s: %v", ref, err)
	}
	defer reader.Close()
	if _, err := io.Copy(io.Discard, reader); err != nil {
		tc.t.Fatalf("reading pull stream for %s: %v", ref, err)
	}
}

// WritePinnedFixture writes the two Pinned manifests of the pinned-policy
// suite into dir: an Application and a Resource both naming the registry's
// shrine/whoami repository without a version. envValue drives the recreate
// cycle; changing it changes the container config hash.
func WritePinnedFixture(tc *TestCase, dir, host, envValue string) {
	tc.t.Helper()
	app := fmt.Sprintf(`apiVersion: shrine/v1
kind: Application
metadata:
  name: whoami-pinned
  owner: shrine-deploy-test
spec:
  image: %s/shrine/whoami
  port: 80
  imagePullPolicy: Pinned
  env:
    - name: ROUND
      value: %q
`, host, envValue)
	resource := fmt.Sprintf(`apiVersion: shrine/v1
kind: Resource
metadata:
  name: cache-pinned
  owner: shrine-deploy-test
spec:
  type: cache
  image: %s/shrine/whoami
  imagePullPolicy: Pinned
`, host)
	writeFixtureFile(tc, filepath.Join(dir, "app.yml"), app)
	writeFixtureFile(tc, filepath.Join(dir, "resource.yml"), resource)
}

// WriteManifestOwnedFixture rewrites the pinned suite's application to a
// fixed tag with no policy, the manifest-owned shape that releases a pin.
func WriteManifestOwnedFixture(tc *TestCase, dir, host, tag string) {
	tc.t.Helper()
	app := fmt.Sprintf(`apiVersion: shrine/v1
kind: Application
metadata:
  name: whoami-pinned
  owner: shrine-deploy-test
spec:
  image: %s/shrine/whoami:%s
  port: 80
`, host, tag)
	writeFixtureFile(tc, filepath.Join(dir, "app.yml"), app)
}

func writeFixtureFile(tc *TestCase, path, content string) {
	tc.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tc.t.Fatalf("creating fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		tc.t.Fatalf("writing fixture %s: %v", path, err)
	}
}
