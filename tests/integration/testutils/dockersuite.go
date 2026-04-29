//go:build integration

package testutils

import (
	"context"
	"fmt"
	stdnet "net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	dockernet "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

func NewDockerSuite(t *testing.T, teamName string) *Suite {
	t.Helper()
	s := NewSuite(t)

	s.BeforeEach(func(tc *TestCase) {
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			tc.t.Fatalf("creating docker client: %v", err)
		}
		tc.DockerClient = cli
		tc.TeamName = teamName
		cleanupDockerResources(tc, teamName)
	})

	s.AfterEach(func(tc *TestCase) {
		if tc.DockerClient == nil {
			return
		}
		cleanupDockerResources(tc, teamName)
		tc.DockerClient.Close()
	})

	return s
}

// SeedSubnetState scans existing Docker networks for 10.100.x.0/24 subnets
// and pre-populates StateDir/subnets.txt so shrine skips those octets when
// allocating a subnet for the test team. Call this from BeforeEach after
// tc.StateDir is set.
func SeedSubnetState(tc *TestCase) {
	if tc.DockerClient == nil || tc.StateDir == "" {
		return
	}
	ctx := context.Background()
	networks, err := tc.DockerClient.NetworkList(ctx, dockernet.ListOptions{})
	if err != nil {
		return
	}
	var lines []string
	for _, n := range networks {
		for _, cfg := range n.IPAM.Config {
			ip, _, err := stdnet.ParseCIDR(cfg.Subnet)
			if err != nil {
				continue
			}
			v4 := ip.To4()
			if v4 == nil || v4[0] != 10 || v4[1] != 100 {
				continue
			}
			lines = append(lines, fmt.Sprintf("_reserved_%s=%s", n.Name, cfg.Subnet))
		}
	}
	if len(lines) == 0 {
		return
	}
	content := strings.Join(lines, "\n") + "\n"
	os.WriteFile(filepath.Join(tc.StateDir, "subnets.txt"), []byte(content), 0644)
}

func cleanupDockerResources(tc *TestCase, teamName string) {
	ctx := context.Background()
	prefix := teamName + "."
	networkName := "shrine." + teamName + ".private"

	containers, _ := tc.DockerClient.ContainerList(ctx, container.ListOptions{All: true})
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.HasPrefix(strings.TrimPrefix(name, "/"), prefix) {
				tc.DockerClient.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true})
				break
			}
		}
	}

	tc.DockerClient.NetworkRemove(ctx, networkName)
}

// CleanupTeam removes all containers and the private network for the given
// team. Use this in BeforeEach when a test suite needs to clean up additional
// teams beyond the primary one managed by NewDockerSuite.
func CleanupTeam(tc *TestCase, teamName string) {
	cleanupDockerResources(tc, teamName)
}
