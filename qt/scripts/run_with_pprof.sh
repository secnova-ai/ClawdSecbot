#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
QT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PPROF_PORT="${1:-${BOTSEC_PPROF_PORT:-6060}}"
BUILD_TYPE="${2:-community}"
BUILD_DIR="${QT_BUILD_DIR:-$QT_ROOT/build/debug}"

if [[ "$BUILD_TYPE" != "community" && "$BUILD_TYPE" != "business" ]]; then
    echo "错误: BUILD_TYPE 仅支持 community 或 business"
    exit 1
fi

QT_PREFIX="${CMAKE_PREFIX_PATH:-}"
if [[ -z "$QT_PREFIX" ]] && command -v brew >/dev/null 2>&1; then
    QT_PREFIX="$(brew --prefix qt 2>/dev/null || true)"
fi

CMAKE_ARGS=(-S "$QT_ROOT" -B "$BUILD_DIR" -DCMAKE_BUILD_TYPE=Debug)
if [[ -n "$QT_PREFIX" ]]; then
    CMAKE_ARGS+=("-DCMAKE_PREFIX_PATH=$QT_PREFIX")
fi

cmake "${CMAKE_ARGS[@]}"
cmake --build "$BUILD_DIR" --parallel
ctest --test-dir "$BUILD_DIR" --output-on-failure

export BOTSEC_PPROF_PORT="$PPROF_PORT"
export BOTSEC_BUILD_TYPE="$BUILD_TYPE"

echo "Qt 客户端启动中，pprof: http://127.0.0.1:${PPROF_PORT}/debug/pprof/"
if [[ "$(uname -s)" == "Darwin" ]]; then
    exec "$BUILD_DIR/ClawdSecbot.app/Contents/MacOS/ClawdSecbot"
fi
exec "$BUILD_DIR/clawdsecbot_qt"
