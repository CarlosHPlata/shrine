//go:build integration

package testutils

import (
	"context"
	"strings"

	"github.com/docker/docker/api/types/network"
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
