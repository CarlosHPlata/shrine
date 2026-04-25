#!/bin/sh
set -e

REPO="CarlosHPlata/shrine"
BINARY="shrine"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Parse flags
VERSION=""
while [ $# -gt 0 ]; do
  case "$1" in
    --version|-v)
      VERSION="$2"
      shift 2
      ;;
    *)
      echo "Unknown flag: $1" >&2
      exit 1
      ;;
  esac
done

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux|darwin) ;;
  msys*|cygwin*|mingw*)
    echo "Windows is not supported. Use WSL2 and run the Linux installer inside it." >&2
    exit 1
    ;;
  *)
    echo "Unsupported OS: $OS" >&2
    exit 1
    ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

# Resolve latest version if not pinned
if [ -z "$VERSION" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | cut -d '"' -f 4)
  if [ -z "$VERSION" ]; then
    echo "Failed to fetch latest version from GitHub API" >&2
    exit 1
  fi
fi

echo "Installing shrine ${VERSION} (${OS}/${ARCH})..."

# Build artifact URL
ARCHIVE="shrine_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

# Download to a temp dir, clean up on exit
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading ${URL} ..."
curl -fsSL "$URL" -o "${TMP}/${ARCHIVE}"

tar -xzf "${TMP}/${ARCHIVE}" -C "$TMP"

# Install — escalate to sudo only when needed
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  chmod +x "${INSTALL_DIR}/${BINARY}"
else
  echo "Writing to ${INSTALL_DIR} requires sudo..."
  sudo mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  sudo chmod +x "${INSTALL_DIR}/${BINARY}"
fi

echo ""
echo "shrine ${VERSION} installed to ${INSTALL_DIR}/${BINARY}"
echo "Run 'shrine version' to verify the installation."
