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

# Release base URL. Override with TOOLNET_RELEASE_BASE if you self-host.
RELEASE_BASE="${TOOLNET_RELEASE_BASE:-https://toolnet.tech/releases}"

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
  if command -v curl >/dev/null 2>&1; then
    VERSION="$(curl -fsS "${RELEASE_BASE}/latest")" || VERSION=""
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
  if [[ -n "${TOOLNET_VERSION:-}" ]]; then
    go build -o "$out" -ldflags "-X main.version=${TOOLNET_VERSION}" "./cmd/toolnet"
  else
    go build -o "$out" "./cmd/toolnet"
  fi
}

install_from_release() {
  local os arch url tmp
  os="$(detect_os)"
  arch="$(detect_arch)"
  url="${RELEASE_BASE}/${VERSION}/${BINARY}-${os}-${arch}"
  echo "Downloading $BINARY ${VERSION} (${os}/${arch})..."
  tmp="$(mktemp)"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$tmp" || err "download failed: $url"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$tmp" "$url" || err "download failed: $url"
  else
    err "need curl or wget to download the release"
  fi
  mv "$tmp" "$INSTALL_DIR/$BINARY"
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
