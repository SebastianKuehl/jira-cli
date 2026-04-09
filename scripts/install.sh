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
TARGET_PATH="$INSTALL_DIR/$BIN_NAME"

echo "[1/6] Starting jira installation for $OS"
echo "[2/6] Repository root: $REPO_ROOT"
echo "[3/6] Install directory: $INSTALL_DIR"
mkdir -p "$INSTALL_DIR"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "[4/6] Building binary..."
(
  cd "$REPO_ROOT"
  go build -o "$TMP_DIR/$BIN_NAME" ./cmd/jira
)

echo "[5/6] Installing binary to $TARGET_PATH"
install -m 0755 "$TMP_DIR/$BIN_NAME" "$TARGET_PATH"

echo "[6/6] Installation complete: $TARGET_PATH"
echo "Run 'jira --help' to verify."
if ! echo ":$PATH:" | grep -q ":$INSTALL_DIR:"; then
  echo "Note: $INSTALL_DIR is not on PATH."
  echo "Add this to your shell profile:"
  echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
fi
