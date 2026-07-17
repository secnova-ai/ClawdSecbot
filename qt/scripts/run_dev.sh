#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
QT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BUILD_DIR="${BUILD_DIR:-$QT_ROOT/build/debug}"
BUILD_TYPE="${BUILD_TYPE:-Debug}"

cmake -S "$QT_ROOT" -B "$BUILD_DIR" -DCMAKE_BUILD_TYPE="$BUILD_TYPE"
cmake --build "$BUILD_DIR" --parallel
ctest --test-dir "$BUILD_DIR" --output-on-failure

if [[ "$(uname -s)" == "Darwin" && -d "$BUILD_DIR/ClawdSecbot.app" ]]; then
  "$BUILD_DIR/ClawdSecbot.app/Contents/MacOS/ClawdSecbot"
else
  "$BUILD_DIR/clawdsecbot_qt"
fi
