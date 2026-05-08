package dockercontainer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/docker/docker/api/types/registry"
)

// hasRegistryAliasPrefix reports whether ref starts with the reg: scheme.
func hasRegistryAliasPrefix(ref string) bool {
	return strings.HasPrefix(ref, "reg:")
}

// expandRegistryAlias replaces a reg:<alias>/ prefix with the corresponding
// registry host. Returns the ref unchanged if no reg: prefix is present.
func expandRegistryAlias(ref string, registries []config.RegistryConfig) (string, error) {
	if !hasRegistryAliasPrefix(ref) {
		return ref, nil
	}
	rest := ref[len("reg:"):]
	slash := strings.Index(rest, "/")
	var alias, tail string
	if slash == -1 {
		alias = rest
		tail = ""
	} else {
		alias = rest[:slash]
		tail = rest[slash:]
	}
	if alias == "" {
		return "", fmt.Errorf("image %q: alias name must not be empty", ref)
	}
	for _, r := range registries {
		if r.Alias == alias {
			return r.Host + tail, nil
		}
	}
	return "", fmt.Errorf("image %q: alias %q is not defined in config registries", ref, alias)
}

// registryAuthFor returns the base64 encoded auth JSON Docker expects
// on ImagePull for the registry implied by the image reference. Returns "" if
// the image targets a registry we have no credentials for (anonymous pull).
func (backend *DockerBackend) registryAuthFor(imageRef string) (string, error) {
	host := parseRegistryHost(imageRef)
	if host == "" {
		return "", nil // Dockerhub
	}

	var match *config.RegistryConfig
	for i := range backend.registries {
		if backend.registries[i].Host == host {
			match = &backend.registries[i]
			break
		}
	}

	if match == nil {
		return "", nil // No credentials for this registry
	}

	return encodeAuth(match)
}

// parseRegistryHost extracts the registry host from a Docker image reference.
//
//	"postgres:16"              -> "" (Docker hub)
//	"library/nginx:latest"     -> "" (Docker hub)
//	"[IP_ADDRESS]/myimage:tag" -> "[IP_ADDRESS]"
//	"ghcr.io/foo/bar:1.0"      -> "ghcr.io"
//	"localhost:5000/foo"       -> "localhost:5000"
func parseRegistryHost(imageRef string) string {
	slash := strings.Index(imageRef, "/")
	if slash == -1 {
		return ""
	}

	prefix := imageRef[:slash]
	// Docker hub namespaces ("library", "bitname", etc.) have no "." or ":".
	if !strings.ContainsAny(prefix, ".:") && prefix != "localhost" {
		return ""
	}

	return prefix
}

func encodeAuth(reg *config.RegistryConfig) (string, error) {
	cfg := registry.AuthConfig{
		Username:      reg.Username,
		Password:      reg.Password,
		ServerAddress: reg.Host,
	}

	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
