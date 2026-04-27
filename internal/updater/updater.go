package updater

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const repo = "CarlosHPlata/shrine"

type Release struct {
	TagName string `json:"tag_name"`
}

// LatestVersion fetches the latest release tag from GitHub.
func LatestVersion() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}

// IsNewer reports whether latest is a newer version than current.
// Both are expected to be semver strings optionally prefixed with "v".
func IsNewer(current, latest string) bool {
	return strings.TrimPrefix(latest, "v") != strings.TrimPrefix(current, "v") &&
		latest != "" && current != "dev"
}

// Update downloads the latest release and replaces the running binary.
func Update(out io.Writer) error {
	latest, err := LatestVersion()
	if err != nil {
		return fmt.Errorf("fetching latest version: %w", err)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("resolving symlinks: %w", err)
	}

	goos := runtime.GOOS
	arch := runtime.GOARCH
	archive := fmt.Sprintf("shrine_%s_%s.tar.gz", goos, arch)
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, latest, archive)

	fmt.Fprintf(out, "Downloading shrine %s (%s/%s)...\n", latest, goos, arch)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("downloading release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	binary, err := extractBinary(resp.Body)
	if err != nil {
		return fmt.Errorf("extracting binary: %w", err)
	}

	tmp, err := os_createTemp(filepath.Dir(exePath), "shrine-update-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os_remove(tmp.Name())

	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	tmp.Close()

	if err := os_chmod(tmp.Name(), 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os_rename(tmp.Name(), exePath); err != nil {
		return fmt.Errorf("replacing binary (try with sudo?): %w", err)
	}

	fmt.Fprintf(out, "shrine updated to %s\n", latest)
	return nil
}

func extractBinary(r io.Reader) ([]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == "shrine" || strings.HasSuffix(hdr.Name, "/shrine") {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("shrine binary not found in archive")
}

// indirections for testing
var (
	os_createTemp = os.CreateTemp
	os_remove     = os.Remove
	os_chmod      = os.Chmod
	os_rename     = os.Rename
)
