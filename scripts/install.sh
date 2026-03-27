#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${PATCHDECK_BIN_DIR:-$HOME/.local/bin}"

mkdir -p "$BIN_DIR"
cd "$ROOT"
go build -o "$BIN_DIR/patchdeck" ./cmd/patchdeck

cat <<EOF
Installed patchdeck to:
  $BIN_DIR/patchdeck

If "$BIN_DIR" is not on your PATH, add:
  export PATH="$BIN_DIR:\$PATH"

Then you can run:
  patchdeck use /path/to/repo
  patchdeck
EOF
