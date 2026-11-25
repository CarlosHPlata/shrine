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
// the registry now serves for the tag.
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

	return r.servedDigest(tc, repoTag)
}

// manifestAccept lists every manifest media type, so the registry answers
// with whatever it stored for the tag (a single manifest or an index).
const manifestAccept = "application/vnd.oci.image.index.v1+json, " +
	"application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.oci.image.manifest.v1+json, " +
	"application/vnd.docker.distribution.manifest.v2+json"

// servedDigest asks the registry which digest it serves for repoTag. The
// daemon's RepoDigests can't be trusted for this: with the containerd image
// store, pushing a single-platform pull of a multi-arch image uploads only
// that platform's manifest, yet RepoDigests still records the upstream
// index digest.
func (r *LocalRegistry) servedDigest(tc *TestCase, repoTag string) string {
	tc.t.Helper()
	repo, tag := splitRepoTag(repoTag)
	url := "http://" + r.Host + "/v2/" + repo + "/manifests/" + tag

	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		tc.t.Fatalf("building manifest request for %s: %v", url, err)
	}
	req.Header.Set("Accept", manifestAccept)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		tc.t.Fatalf("requesting manifest %s: %v", url, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tc.t.Fatalf("manifest %s: registry answered %s", url, resp.Status)
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		tc.t.Fatalf("manifest %s: registry sent no Docker-Content-Digest header", url)
	}
	return digest
}

func splitRepoTag(repoTag string) (repo, tag string) {
	if colon := strings.LastIndex(repoTag, ":"); colon > strings.LastIndex(repoTag, "/") {
		return repoTag[:colon], repoTag[colon+1:]
	}
	return repoTag, "latest"
}

// ImageLayersOf returns the root filesystem layer digests of ref (an image
// reference or id), failing the test when absent. Layers identify an image's
// content under both image stores. Image ids don't: with the containerd
// store, a multi-arch pull's id is its index digest, while a container
// created from the platform manifest reports that manifest's digest.
func (tc *TestCase) ImageLayersOf(ref string) []string {
	tc.t.Helper()
	inspected, err := tc.DockerClient.ImageInspect(context.Background(), ref)
	if err != nil {
		tc.t.Fatalf("inspecting image %s: %v", ref, err)
	}
	return inspected.RootFS.Layers
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
