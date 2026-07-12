#!/usr/bin/env bash
#
# TOOLNET CLI installer.
#
# Usage:
#   curl https://toolnet.tech/install | bash
#   curl https://toolnet.tech/install | bash -s -- --version v1.2.3
#   ./install.sh                 # install latest
#   ./install.sh --from-source   # build from the local repo instead of downloading
#
# Installs the `toolnet` binary into INSTALL_DIR (default: /usr/local/bin,
# or ~/.local/bin if /usr/local/bin is not writable) and makes it executable.

set -euo pipefail

REPO="LBT-AI/toolnet.tech"
BINARY="toolnet"
INSTALL_DIR="${INSTALL_DIR:-}"
VERSION="latest"
FROM_SOURCE=0

# GitHub Releases is the canonical binary source. Override for a mirror.
RELEASE_BASE="${TOOLNET_RELEASE_BASE:-https://github.com/${REPO}/releases}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --from-source) FROM_SOURCE=1; shift ;;
    --dir) INSTALL_DIR="$2"; shift 2 ;;
    -h|--help)
      grep '^#' "$0" | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

err() { echo "error: $*" >&2; exit 1; }

detect_os() {
  local os
  os="$(uname -s)"
  case "$os" in
    Linux) echo "linux" ;;
    Darwin) echo "darwin" ;;
    *) err "unsupported OS: $os" ;;
  esac
}

detect_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) err "unsupported architecture: $arch" ;;
  esac
}

choose_install_dir() {
  if [[ -n "$INSTALL_DIR" ]]; then
    mkdir -p "$INSTALL_DIR"
    return
  fi
  if [[ -w /usr/local/bin ]]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="${HOME}/.local/bin"
    mkdir -p "$INSTALL_DIR"
    echo "Note: /usr/local/bin not writable; installing to $INSTALL_DIR"
    echo "Make sure $INSTALL_DIR is on your PATH."
  fi
}

resolve_version() {
  if [[ "$VERSION" != "latest" ]]; then
    return
  fi
  local latest_url="https://api.github.com/repos/${REPO}/releases/latest"
  if command -v curl >/dev/null 2>&1; then
    VERSION="$(curl -fsSL "$latest_url" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)" || VERSION=""
  elif command -v wget >/dev/null 2>&1; then
    VERSION="$(wget -qO- "$latest_url" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)" || VERSION=""
  fi
  if [[ -z "$VERSION" ]]; then
    # Fall back to building from source.
    FROM_SOURCE=1
  fi
}

install_from_source() {
  echo "Building $BINARY from source..."
  if ! command -v go >/dev/null 2>&1; then
    err "go is required to build from source (https://go.dev/dl/)"
  fi
  local out="$INSTALL_DIR/$BINARY"
  if [[ -f "go.mod" && -d "cmd/toolnet" ]]; then
    if [[ -n "${TOOLNET_VERSION:-}" ]]; then
      go build -o "$out" -ldflags "-X main.version=${TOOLNET_VERSION}" "./cmd/toolnet"
    else
      go build -o "$out" "./cmd/toolnet"
    fi
  else
    # curl .../install | bash runs outside a source checkout. Let Go fetch
    # the public module directly and place the binary in the chosen directory.
    local module_version="latest"
    if [[ "$VERSION" != "latest" ]]; then
      module_version="$VERSION"
    fi
    GOBIN="$INSTALL_DIR" go install "github.com/${REPO}/cmd/toolnet@${module_version}"
  fi
}

install_from_release() {
  local os arch asset url checksum_url tmp checksums expected actual
  os="$(detect_os)"
  arch="$(detect_arch)"
  asset="${BINARY}-${os}-${arch}"
  url="${RELEASE_BASE}/download/${VERSION}/${asset}"
  checksum_url="${RELEASE_BASE}/download/${VERSION}/checksums.txt"
  echo "Downloading $BINARY ${VERSION} (${os}/${arch})..."
  tmp="$(mktemp)"
  checksums="$(mktemp)"
  trap 'rm -f "$tmp" "$checksums"' EXIT
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$tmp" || err "download failed: $url"
    curl -fsSL "$checksum_url" -o "$checksums" || err "checksum download failed"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$tmp" "$url" || err "download failed: $url"
    wget -qO "$checksums" "$checksum_url" || err "checksum download failed"
  else
    err "need curl or wget to download the release"
  fi
  expected="$(awk -v name="$asset" '$2 == name {print $1}' "$checksums")"
  [[ -n "$expected" ]] || err "no checksum found for $asset"
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$tmp" | awk '{print $1}')"
  else
    actual="$(shasum -a 256 "$tmp" | awk '{print $1}')"
  fi
  [[ "$actual" == "$expected" ]] || err "checksum verification failed for $asset"
  mv "$tmp" "$INSTALL_DIR/$BINARY"
  trap - EXIT
  rm -f "$checksums"
}

main() {
  choose_install_dir
  if [[ "$FROM_SOURCE" -eq 1 ]]; then
    install_from_source
  else
    resolve_version
    if [[ "$FROM_SOURCE" -eq 1 ]]; then
      install_from_source
    else
      install_from_release
    fi
  fi
  chmod +x "$INSTALL_DIR/$BINARY"
  echo
  echo "$BINARY installed to $INSTALL_DIR/$BINARY"
  if ! command -v "$BINARY" >/dev/null 2>&1; then
    echo "Reminder: add $INSTALL_DIR to your PATH if it is not already."
  fi
  echo
  echo "Next steps:"
  echo "  $BINARY config            # validate your config"
  echo "  $BINARY login --provider openai   # authenticate (device flow)"
  echo "  $BINARY                   # start an interactive session"
}

main "$@"
