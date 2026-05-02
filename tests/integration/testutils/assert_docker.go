//go:build integration

package testutils

import (
	"context"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

func (tc *TestCase) AssertContainerRunning(name string) *TestCase {
	tc.t.Helper()
	ctx := context.Background()
	info, err := tc.DockerClient.ContainerInspect(ctx, name)
	if err != nil {
		tc.t.Errorf("container %q not found: %v", name, err)
		return tc
	}
	if info.State.Status != "running" {
		tc.t.Errorf("container %q status = %q, want \"running\"", name, info.State.Status)
	}
	return tc
}

func (tc *TestCase) AssertContainerNotExists(name string) *TestCase {
	tc.t.Helper()
	ctx := context.Background()
	_, err := tc.DockerClient.ContainerInspect(ctx, name)
	if err == nil {
		tc.t.Errorf("expected container %q to NOT exist", name)
	}
	return tc
}

func (tc *TestCase) AssertContainerHasBindMount(containerName, hostSource, target string) *TestCase {
	tc.t.Helper()
	ctx := context.Background()
	info, err := tc.DockerClient.ContainerInspect(ctx, containerName)
	if err != nil {
		tc.t.Errorf("container %q not found: %v", containerName, err)
		return tc
	}
	for _, m := range info.Mounts {
		if m.Type == "bind" && m.Source == hostSource && m.Destination == target {
			return tc
		}
	}
	tc.t.Errorf("container %q missing bind mount %s → %s\nmounts: %+v", containerName, hostSource, target, info.Mounts)
	return tc
}

func (tc *TestCase) RemoveContainerIfExists(name string) {
	tc.t.Helper()
	ctx := context.Background()
	_, err := tc.DockerClient.ContainerInspect(ctx, name)
	if err != nil {
		return
	}
	_ = tc.DockerClient.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
}

func (tc *TestCase) AssertNetworkExists(name string) *TestCase {
	tc.t.Helper()
	ctx := context.Background()
	_, err := tc.DockerClient.NetworkInspect(ctx, name, network.InspectOptions{})
	if err != nil {
		tc.t.Errorf("network %q not found: %v", name, err)
	}
	return tc
}

func (tc *TestCase) AssertContainerInNetwork(containerName, networkName string) *TestCase {
	tc.t.Helper()
	ctx := context.Background()
	info, err := tc.DockerClient.ContainerInspect(ctx, containerName)
	if err != nil {
		tc.t.Errorf("container %q not found: %v", containerName, err)
		return tc
	}
	if _, ok := info.NetworkSettings.Networks[networkName]; !ok {
		tc.t.Errorf("container %q is not attached to network %q", containerName, networkName)
	}
	return tc
}

func (tc *TestCase) AssertContainerEnvVar(containerName, key, expectedValue string) *TestCase {
	tc.t.Helper()
	ctx := context.Background()
	info, err := tc.DockerClient.ContainerInspect(ctx, containerName)
	if err != nil {
		tc.t.Errorf("container %q not found: %v", containerName, err)
		return tc
	}
	for _, entry := range info.Config.Env {
		k, v, _ := strings.Cut(entry, "=")
		if k == key {
			if v != expectedValue {
				tc.t.Errorf("container %q env %q = %q, want %q", containerName, key, v, expectedValue)
			}
			return tc
		}
	}
	tc.t.Errorf("container %q missing env var %q", containerName, key)
	return tc
}

func (tc *TestCase) AssertContainerEnvVarNotEmpty(containerName, key string) *TestCase {
	tc.t.Helper()
	ctx := context.Background()
	info, err := tc.DockerClient.ContainerInspect(ctx, containerName)
	if err != nil {
		tc.t.Errorf("container %q not found: %v", containerName, err)
		return tc
	}
	for _, entry := range info.Config.Env {
		k, v, _ := strings.Cut(entry, "=")
		if k == key {
			if v == "" {
				tc.t.Errorf("container %q env %q is empty", containerName, key)
			}
			return tc
		}
	}
	tc.t.Errorf("container %q missing env var %q", containerName, key)
	return tc
}

func (tc *TestCase) AssertContainerNotRunning(name string) *TestCase {
	tc.t.Helper()
	ctx := context.Background()
	info, err := tc.DockerClient.ContainerInspect(ctx, name)
	if err != nil {
		// Container not found — that's the expected state
		return tc
	}
	if info.State.Status == "running" {
		tc.t.Errorf("container %q is still running, expected it to be stopped or removed", name)
	}
	return tc
}

func (tc *TestCase) AssertNetworkNotExists(name string) *TestCase {
	tc.t.Helper()
	ctx := context.Background()
	_, err := tc.DockerClient.NetworkInspect(ctx, name, network.InspectOptions{})
	if err == nil {
		tc.t.Errorf("network %q still exists, expected it to be removed", name)
	}
	return tc
}

func (tc *TestCase) AssertContainerPublishesPort(name, hostPort, containerPort, proto string) *TestCase {
	tc.t.Helper()
	ctx := context.Background()
	info, err := tc.DockerClient.ContainerInspect(ctx, name)
	if err != nil {
		tc.t.Fatalf("container %q not found: %v", name, err)
	}
	key := nat.Port(containerPort + "/" + proto)
	bindings, exists := info.HostConfig.PortBindings[key]
	if exists {
		for _, binding := range bindings {
			if binding.HostPort == hostPort {
				return tc
			}
		}
	}
	// Binding not found — list all actual bindings for debugging
	var allBindings []string
	for k, v := range info.HostConfig.PortBindings {
		for _, binding := range v {
			allBindings = append(allBindings, "  "+string(k)+" → HostPort:"+binding.HostPort)
		}
	}
	var bindingStr string
	if len(allBindings) > 0 {
		bindingStr = "\nActual bindings:\n" + strings.Join(allBindings, "\n")
	} else {
		bindingStr = "\nNo port bindings found"
	}
	tc.t.Fatalf("container %q missing port binding %s → %s:%s%s", name, hostPort, containerPort, proto, bindingStr)
	return tc
}
