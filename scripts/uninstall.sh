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

INSTALL_DIR="${JIRA_INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="jira"
TARGET_PATH="$INSTALL_DIR/$BIN_NAME"

echo "[1/3] Starting jira uninstall for $OS"
echo "[2/3] Target binary: $TARGET_PATH"
if [[ -f "$TARGET_PATH" ]]; then
  rm -f "$TARGET_PATH"
  echo "[3/3] Removed: $TARGET_PATH"
else
  echo "[3/3] Nothing to remove (file not found)."
fi
