#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_DIR="$ROOT_DIR/backend_go/build/bin"
EXPORT_DIR="$ROOT_DIR/export_app"
APP_NAME="MeshHub"

mkdir -p "$EXPORT_DIR"

if [ ! -d "$SOURCE_DIR" ]; then
  echo "Wails build output not found: $SOURCE_DIR" >&2
  exit 1
fi

find "$EXPORT_DIR" -mindepth 1 ! -name ".gitkeep" -exec rm -rf {} +
find "$SOURCE_DIR" -maxdepth 1 -name "asset-transcoder*" -exec rm -rf {} +

if [ -d "$SOURCE_DIR/$APP_NAME.app" ]; then
  cp -R "$SOURCE_DIR/$APP_NAME.app" "$EXPORT_DIR"/
fi

if [ -f "$SOURCE_DIR/$APP_NAME.exe" ]; then
  cp "$SOURCE_DIR/$APP_NAME.exe" "$EXPORT_DIR"/
fi

echo "Exported Wails build to $EXPORT_DIR"
