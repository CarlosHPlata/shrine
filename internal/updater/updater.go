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
	"strconv"
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

// IsNewer reports whether latest is a strictly newer version than current,
// using SemVer 2.0.0 precedence rules. Both are expected to be semver
// strings optionally prefixed with "v". A "dev" current version, an empty
// latest, or unparseable inputs all yield false.
func IsNewer(current, latest string) bool {
	if current == "dev" || latest == "" {
		return false
	}
	return compareSemver(latest, current) > 0
}

// compareSemver returns >0 if a > b, <0 if a < b, 0 if equal or either
// side fails to parse.
func compareSemver(a, b string) int {
	aMain, aPre, aOK := parseSemver(a)
	bMain, bPre, bOK := parseSemver(b)
	if !aOK || !bOK {
		return 0
	}
	for i := 0; i < 3; i++ {
		if aMain[i] != bMain[i] {
			if aMain[i] > bMain[i] {
				return 1
			}
			return -1
		}
	}
	// A version with a pre-release has lower precedence than one without.
	switch {
	case aPre == "" && bPre == "":
		return 0
	case aPre == "":
		return 1
	case bPre == "":
		return -1
	}
	return comparePrerelease(aPre, bPre)
}

// parseSemver splits a semver string into its three numeric components and
// pre-release tail. Build metadata (after '+') is discarded per SemVer.
func parseSemver(s string) ([3]int, string, bool) {
	s = strings.TrimPrefix(s, "v")
	if j := strings.IndexByte(s, '+'); j >= 0 {
		s = s[:j]
	}
	var pre string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return [3]int{}, "", false
	}
	var nums [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, "", false
		}
		nums[i] = n
	}
	return nums, pre, true
}

// comparePrerelease applies SemVer 2.0.0 pre-release precedence:
// numeric identifiers compare numerically and rank below alphanumeric ones;
// alphanumeric identifiers compare in ASCII order; a longer set of
// identifiers wins when all preceding identifiers are equal.
func comparePrerelease(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	n := len(aParts)
	if len(bParts) < n {
		n = len(bParts)
	}
	for i := 0; i < n; i++ {
		aN, aErr := strconv.Atoi(aParts[i])
		bN, bErr := strconv.Atoi(bParts[i])
		aIsNum := aErr == nil
		bIsNum := bErr == nil
		switch {
		case aIsNum && bIsNum:
			if aN != bN {
				if aN > bN {
					return 1
				}
				return -1
			}
		case aIsNum:
			return -1
		case bIsNum:
			return 1
		default:
			if aParts[i] != bParts[i] {
				if aParts[i] > bParts[i] {
					return 1
				}
				return -1
			}
		}
	}
	if len(aParts) == len(bParts) {
		return 0
	}
	if len(aParts) > len(bParts) {
		return 1
	}
	return -1
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
