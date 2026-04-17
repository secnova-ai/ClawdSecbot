#!/bin/bash
# run_web_with_pprof.sh — Build Flutter Web bundle and serve via Go web bridge (same-origin).
#
# Usage:
#   ./scripts/run_web_with_pprof.sh
#   ./scripts/run_web_with_pprof.sh 6061
#
# Environment:
#   BOTSEC_WEB_API_PORT=18080         # Go bridge + Web UI main port
#   BOTSEC_WEB_API_HOST=0.0.0.0
#   BOTSEC_WORKSPACE_DIR_PREFIX=/opt/botsec/workspace
#   BOTSEC_HOME_DIR=/home/botsec
#   BOTSEC_CURRENT_VERSION=1.0.1
set -euo pipefail

PROJECT_ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." &> /dev/null && pwd )"
cd "$PROJECT_ROOT"

BOTSEC_APP_SUPPORT_ID="${BOTSEC_APP_SUPPORT_ID:-com.bot.secnova.clawdsecbot}"

default_workspace_prefix() {
    case "$OSTYPE" in
        darwin*)
            echo "$HOME/Library/Application Support/$BOTSEC_APP_SUPPORT_ID"
            ;;
        linux-gnu*)
            echo "${XDG_DATA_HOME:-$HOME/.local/share}/$BOTSEC_APP_SUPPORT_ID"
            ;;
        msys*|win32*)
            echo "${APPDATA:-$HOME}/$BOTSEC_APP_SUPPORT_ID"
            ;;
        *)
            echo "$PROJECT_ROOT/.botsec_web_workspace"
            ;;
    esac
}

PPROF_PORT="${1:-${BOTSEC_PPROF_PORT:-6060}}"
API_PORT="${BOTSEC_WEB_API_PORT:-18080}"
API_HOST="${BOTSEC_WEB_API_HOST:-0.0.0.0}"
WORKSPACE_DIR_PREFIX="${BOTSEC_WORKSPACE_DIR_PREFIX:-$(default_workspace_prefix)}"
HOME_DIR="${BOTSEC_HOME_DIR:-$HOME}"
CURRENT_VERSION="${BOTSEC_CURRENT_VERSION:-1.0.1}"
WEB_STATIC_DIR="${BOTSEC_WEB_STATIC_DIR:-$PROJECT_ROOT/build/web}"

BOTSEC_WEBD_PID=""

is_port_in_use() {
    local port="$1"
    lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
}

run_with_sudo() {
    if sudo -n true 2>/dev/null; then
        sudo -n "$@"
    else
        sudo "$@"
    fi
}

cleanup() {
    if [[ -n "${BOTSEC_WEBD_PID:-}" ]] && kill -0 "$BOTSEC_WEBD_PID" 2>/dev/null; then
        kill "$BOTSEC_WEBD_PID" 2>/dev/null || true
        wait "$BOTSEC_WEBD_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT INT TERM

echo "============================================"
echo "  BotSecManager Web Debug (pprof mode)"
echo "============================================"
echo "pprof port:  $PPROF_PORT"
echo "api+web:     $API_HOST:$API_PORT"
echo ""

if is_port_in_use "$API_PORT"; then
    echo "error: API/Web port already in use: $API_PORT"
    echo "set BOTSEC_WEB_API_PORT to another port or stop existing process"
    exit 1
fi

if is_port_in_use "$PPROF_PORT"; then
    echo "warning: pprof port already in use: $PPROF_PORT"
    echo "pprof may fail to start; set BOTSEC_PPROF_PORT to another port if needed"
fi

echo "[1/4] Build plugin..."
"$PROJECT_ROOT/scripts/build_openclaw_plugin.sh"
echo ""

if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "[2/4] Build and install libsandbox_preload.so..."
    SANDBOX_DIR="$PROJECT_ROOT/go_lib/core/sandbox/linux_hook"
    POLICY_DIR="${HOME}/.botsec/policies"
    PRELOAD_SO="$SANDBOX_DIR/build/libsandbox_preload.so"
    SYSTEM_LIB_DIR="/usr/lib/clawdsecbot"
    SYSTEM_PRELOAD_SO="$SYSTEM_LIB_DIR/libsandbox_preload.so"

    if [[ -d "$SANDBOX_DIR" ]]; then
        mkdir -p "$SANDBOX_DIR/build"
        cmake -S "$SANDBOX_DIR" -B "$SANDBOX_DIR/build" -DCMAKE_BUILD_TYPE=Release
        cmake --build "$SANDBOX_DIR/build" --config Release
        if [[ -f "$PRELOAD_SO" ]]; then
            mkdir -p "$POLICY_DIR"
            cp -f "$PRELOAD_SO" "$POLICY_DIR/libsandbox_preload.so"
            run_with_sudo mkdir -p "$SYSTEM_LIB_DIR"
            run_with_sudo install -m 0755 "$PRELOAD_SO" "$SYSTEM_PRELOAD_SO"
            echo "installed: $POLICY_DIR/libsandbox_preload.so"
            echo "installed: $SYSTEM_PRELOAD_SO"
        else
            echo "warning: preload binary not found: $PRELOAD_SO"
        fi
    else
        echo "warning: sandbox source not found: $SANDBOX_DIR"
    fi
else
    echo "[2/4] Skip preload build (non-Linux)"
fi
echo ""

echo "[3/4] Build Flutter Web bundle..."
if [[ ! -f "$PROJECT_ROOT/.dart_tool/package_config.json" ]]; then
    flutter pub get
fi

flutter build web \
    --target lib/main_web.dart \
    --no-tree-shake-icons \
    --no-wasm-dry-run \
    --dart-define=BOTSEC_WEB_API_PORT="$API_PORT" \
    --dart-define=BOTSEC_WORKSPACE_DIR_PREFIX="$WORKSPACE_DIR_PREFIX" \
    --dart-define=BOTSEC_HOME_DIR="$HOME_DIR" \
    --dart-define=BOTSEC_CURRENT_VERSION="$CURRENT_VERSION"
echo ""

echo "[4/4] Start Go web bridge (API + static web)..."
export BOTSEC_PPROF_PORT="$PPROF_PORT"
(
    cd "$PROJECT_ROOT/go_lib"
    BOTSEC_WEB_STATIC_DIR="$WEB_STATIC_DIR" \
    go run ./cmd/botsec_webd --addr "${API_HOST}:${API_PORT}" --web-root "$WEB_STATIC_DIR"
) &
BOTSEC_WEBD_PID=$!

HEALTH_HOST="$API_HOST"
if [[ "$HEALTH_HOST" == "0.0.0.0" ]]; then
    HEALTH_HOST="127.0.0.1"
fi

for _ in {1..80}; do
    if curl -fsS "http://${HEALTH_HOST}:${API_PORT}/health" >/dev/null 2>&1; then
        break
    fi
    sleep 0.25
done

if ! curl -fsS "http://${HEALTH_HOST}:${API_PORT}/health" >/dev/null 2>&1; then
    echo "error: web bridge failed to become healthy on ${HEALTH_HOST}:${API_PORT}"
    exit 1
fi

echo "Go web bridge ready: http://127.0.0.1:${API_PORT}"
echo "pprof endpoint:      http://127.0.0.1:${PPROF_PORT}/debug/pprof/"
echo "Web UI local URL:    http://127.0.0.1:${API_PORT}"
if [[ "$API_HOST" == "0.0.0.0" ]]; then
    echo "Web UI remote URL:   http://<server-ip>:${API_PORT}"
fi
echo ""
echo "Press Ctrl+C to stop."

wait "$BOTSEC_WEBD_PID"
