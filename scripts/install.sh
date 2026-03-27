#!/usr/bin/env bash
set -euo pipefail

REPO="${PATCHDECK_GITHUB_REPO:-thumbsu/patchdeck}"
BIN_DIR="${PATCHDECK_BIN_DIR:-$HOME/.local/bin}"
MODE="${1:-release}"

detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "$os" in
    darwin|linux) ;;
    *)
      echo "Unsupported OS: $os" >&2
      return 1
      ;;
  esac

  case "$arch" in
    arm64|aarch64) arch="arm64" ;;
    x86_64|amd64) arch="amd64" ;;
    *)
      echo "Unsupported architecture: $arch" >&2
      return 1
      ;;
  esac

  printf '%s %s\n' "$os" "$arch"
}

install_from_release() {
  read -r os arch < <(detect_platform)
  local asset="patchdeck-${os}-${arch}.tar.gz"
  local url="https://github.com/${REPO}/releases/latest/download/${asset}"
  local tmpdir
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "'"$tmpdir"'"' EXIT

  mkdir -p "$BIN_DIR"
  curl -fsSL "$url" -o "$tmpdir/$asset"
  tar -xzf "$tmpdir/$asset" -C "$tmpdir"
  install -m 0755 "$tmpdir/patchdeck-${os}-${arch}" "$BIN_DIR/patchdeck"
}

install_from_source() {
  local root
  root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  mkdir -p "$BIN_DIR"
  cd "$root"
  go build -o "$BIN_DIR/patchdeck" ./cmd/patchdeck
}

case "$MODE" in
  release)
    install_from_release
    ;;
  --build-from-source|source)
    install_from_source
    ;;
  *)
    echo "Usage: ./scripts/install.sh [release|--build-from-source]" >&2
    exit 1
    ;;
esac

cat <<EOF
Installed patchdeck to:
  $BIN_DIR/patchdeck

If "$BIN_DIR" is not on your PATH, add:
  export PATH="$BIN_DIR:\$PATH"

Then you can run:
  patchdeck use /path/to/repo
  patchdeck
EOF
