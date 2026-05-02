---
title: "Install"
description: "Install the Shrine CLI on Linux or macOS."
weight: 10
---

## Prerequisites

- Docker daemon running on the target machine.
- Linux or macOS (Windows is not supported; use WSL2 and follow the Linux path).
- Write access to the install directory — either root or membership in the `docker` group is required to run Shrine commands against the Docker daemon.

## Method 1: install script

The quickest path is the one-liner install script. It detects your OS and architecture, downloads the correct pre-built binary from the latest GitHub release, and places it in `/usr/local/bin`.

```bash
curl -fsSL https://raw.githubusercontent.com/CarlosHPlata/shrine/main/install.sh | sh
```

**Security note**: read the script first if you're cautious — it's plain shell and downloads from the official GitHub releases API.

Override the install directory:

```bash
INSTALL_DIR=~/.local/bin curl -fsSL https://raw.githubusercontent.com/CarlosHPlata/shrine/main/install.sh | sh
```

Pin to a specific release version:

```bash
curl -fsSL https://raw.githubusercontent.com/CarlosHPlata/shrine/main/install.sh | sh -s -- --version v0.1.0
```

## Method 2: `go install`

If you have Go 1.24 or later installed:

```bash
go install github.com/CarlosHPlata/shrine@latest
```

The binary is placed in `$(go env GOPATH)/bin`. Make sure that directory is on your `$PATH`.

## Method 3: download a release binary

Pre-built archives are available on the [GitHub Releases page](https://github.com/CarlosHPlata/shrine/releases). Supported platforms:

| OS | Architecture | Archive name |
|----|--------------|--------------|
| Linux | x86_64 (amd64) | `shrine_linux_amd64.tar.gz` |
| Linux | arm64 | `shrine_linux_arm64.tar.gz` |
| macOS | x86_64 (amd64) | `shrine_darwin_amd64.tar.gz` |
| macOS | Apple Silicon (arm64) | `shrine_darwin_arm64.tar.gz` |

Download the archive for your platform, extract it, and place the `shrine` binary somewhere on your `$PATH`:

```bash
tar -xzf shrine_linux_amd64.tar.gz
sudo mv shrine /usr/local/bin/shrine
```

A `checksums.txt` (SHA-256) is published alongside each release for verification.

## Verify the install

```bash
shrine version
```

Expected output shape:

```text
shrine v0.x.y (commit abc1234, built 2026-05-02)
```

If you see `command not found`, see the troubleshooting notes below.

## Next steps

Continue to [Quick Start](/getting-started/quick-start/) to deploy your first application.

## Troubleshooting

- **`command not found` after install**: the install directory is not on your `$PATH`. See [PATH configuration](/troubleshooting/) for instructions on adding a directory to your shell profile.
- **Docker daemon errors on first run**: your user may not be in the `docker` group, or the Docker daemon may not be running. See [Docker access](/troubleshooting/) for group membership setup and daemon startup steps.
