#!/usr/bin/env bash
set -euo pipefail

OS="$(uname -s)"
case "$OS" in
Darwin|Linux) ;;
*)
  echo "Unsupported OS: $OS (this script supports macOS/Linux)" >&2
  exit 1
  ;;
esac

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
INSTALL_DIR="${JIRA_INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="jira"

echo "Building $BIN_NAME from $REPO_ROOT..."
mkdir -p "$INSTALL_DIR"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

(
  cd "$REPO_ROOT"
  go build -o "$TMP_DIR/$BIN_NAME" ./cmd/jira
)

install -m 0755 "$TMP_DIR/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"

echo "Installed to: $INSTALL_DIR/$BIN_NAME"
if ! echo ":$PATH:" | grep -q ":$INSTALL_DIR:"; then
  echo "Note: $INSTALL_DIR is not on PATH."
  echo "Add this to your shell profile:"
  echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
fi
