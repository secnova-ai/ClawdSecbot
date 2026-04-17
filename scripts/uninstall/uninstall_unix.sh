#!/usr/bin/env bash

# 开启严格模式，保证脚本在异常时快速失败。
set -euo pipefail

# 统一日志输出函数，关键日志使用英文。
log_info() {
    echo "INFO: $1"
}

log_warn() {
    echo "WARN: $1"
}

log_error() {
    echo "ERROR: $1" >&2
}

# 全局参数默认值。
DRY_RUN=false
FORCE=false
REMOVE_SYSTEM_FILES=false
TARGET_PLATFORM="auto"
CUSTOM_INSTALL_PATHS=()

# 已收集待删除路径，使用换行分隔避免空格路径问题。
DELETE_TARGETS=""

# 展示脚本帮助信息。
show_help() {
    cat <<'EOF'
Usage: ./scripts/uninstall/uninstall_unix.sh [options]

Clean ClawdSecbot files on macOS/Linux.

Options:
  --platform <auto|macos|linux>   Target platform (default: auto)
  --install-path <path>            Add custom install path (repeatable)
  --remove-system-files            Remove system package files (Linux/macOS)
  --dry-run                        Show files to delete only
  --force                          Skip interactive confirmation
  -h, --help                       Show help

Cleanup scope:
  [Common]
    - ~/.botsec
    - ~/Documents/.botsec
    - /tmp/botsec, ${TMPDIR}/botsec
    - ${TMPDIR}/clawdsecbot.lock, ${TMPDIR}/clawdsecbot.sock
    - Known runtime directories (no full recursive scan)
    - Any path passed by --install-path
  [macOS]
    - /Applications/ClawdSecbot.app, ~/Applications/ClawdSecbot.app
    - ~/Library/Application Support/{ClawdSecbot,com.clawdsecbot.guard,com.bot.secnova.clawdsecbot}
    - ~/Library/Preferences/com.bot.secnova.clawdsecbot.plist
    - ~/Library/Caches/com.bot.secnova.clawdsecbot
    - ~/Library/Saved Application State/com.bot.secnova.clawdsecbot.savedState
  [Linux]
    - ~/.local/share/{clawdsecbot,com.clawdsecbot.guard,com.bot.secnova.clawdsecbot}
    - ~/.config/{clawdsecbot,com.clawdsecbot.guard,com.bot.secnova.clawdsecbot}
    - ~/.cache/clawdsecbot
    - ~/.local/share/applications/com.clawdsecbot.guard.desktop
  [System level, with --remove-system-files]
    - macOS: /Applications/ClawdSecbot.app
    - Linux: /usr/bin/clawdsecbot, /usr/lib/clawdsecbot,
             /usr/share/applications/clawdsecbot.desktop, /usr/share/pixmaps/clawdsecbot.png

Examples:
  ./scripts/uninstall/uninstall_unix.sh --dry-run
  ./scripts/uninstall/uninstall_unix.sh --platform macos --force
  ./scripts/uninstall/uninstall_unix.sh --platform linux --remove-system-files
EOF
}

# 解析命令行参数。
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --platform)
                TARGET_PLATFORM="${2:-}"
                shift 2
                ;;
            --install-path)
                CUSTOM_INSTALL_PATHS+=("${2:-}")
                shift 2
                ;;
            --remove-system-files)
                REMOVE_SYSTEM_FILES=true
                shift
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --force)
                FORCE=true
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# 根据当前系统归一化平台名称。
resolve_platform() {
    if [[ "$TARGET_PLATFORM" == "auto" ]]; then
        case "$(uname -s)" in
            Darwin)
                TARGET_PLATFORM="macos"
                ;;
            Linux)
                TARGET_PLATFORM="linux"
                ;;
            *)
                log_error "Unsupported platform: $(uname -s)"
                exit 1
                ;;
        esac
    fi

    if [[ "$TARGET_PLATFORM" != "macos" && "$TARGET_PLATFORM" != "linux" ]]; then
        log_error "Invalid platform: $TARGET_PLATFORM"
        exit 1
    fi
}

# 向删除列表追加路径（自动去重、过滤空值）。
add_target() {
    local path="$1"
    [[ -n "${path// }" ]] || return 0
    if [[ -z "$DELETE_TARGETS" ]]; then
        DELETE_TARGETS="$path"
        return 0
    fi
    if ! printf '%s\n' "$DELETE_TARGETS" | awk -v p="$path" 'BEGIN{found=0} $0==p{found=1} END{exit(found?0:1)}'; then
        DELETE_TARGETS="${DELETE_TARGETS}"$'\n'"$path"
    fi
}

# 收集跨平台通用清理目标。
collect_common_targets() {
    add_target "$HOME/.botsec"
    add_target "$HOME/Documents/.botsec"
    add_target "/tmp/botsec"
    add_target "${TMPDIR:-/tmp}/botsec"
    add_target "${TMPDIR:-/tmp}/clawdsecbot.lock"
    add_target "${TMPDIR:-/tmp}/clawdsecbot.sock"

    for custom_path in "${CUSTOM_INSTALL_PATHS[@]}"; do
        add_target "$custom_path"
    done
}

# 收集 macOS 平台目标。
collect_macos_targets() {
    add_target "/Applications/ClawdSecbot.app"
    add_target "$HOME/Applications/ClawdSecbot.app"
    add_target "$HOME/Library/Application Support/ClawdSecbot"
    add_target "$HOME/Library/Application Support/com.clawdsecbot.guard"
    add_target "$HOME/Library/Application Support/com.bot.secnova.clawdsecbot"
    add_target "$HOME/Library/Preferences/com.bot.secnova.clawdsecbot.plist"
    add_target "$HOME/Library/Caches/com.bot.secnova.clawdsecbot"
    add_target "$HOME/Library/Saved Application State/com.bot.secnova.clawdsecbot.savedState"

}

# 收集 Linux 平台目标。
collect_linux_targets() {
    add_target "$HOME/.local/share/clawdsecbot"
    add_target "$HOME/.local/share/com.clawdsecbot.guard"
    add_target "$HOME/.local/share/com.bot.secnova.clawdsecbot"
    add_target "$HOME/.config/clawdsecbot"
    add_target "$HOME/.config/com.clawdsecbot.guard"
    add_target "$HOME/.config/com.bot.secnova.clawdsecbot"
    add_target "$HOME/.cache/clawdsecbot"

    add_target "$HOME/.local/share/applications/com.clawdsecbot.guard.desktop"
    add_target "$HOME/.local/share/icons/hicolor"

}

# 收集系统级安装路径（需要 root 权限）。
collect_system_targets() {
    if [[ "$TARGET_PLATFORM" == "macos" ]]; then
        add_target "/Applications/ClawdSecbot.app"
        return 0
    fi

    add_target "/usr/bin/clawdsecbot"
    add_target "/usr/lib/clawdsecbot"
    add_target "/usr/share/applications/clawdsecbot.desktop"
    add_target "/usr/share/pixmaps/clawdsecbot.png"
    add_target "/usr/share/icons/hicolor"
}

# 停止可能正在运行的 ClawdSecbot 进程。
stop_processes() {
    if command -v pkill >/dev/null 2>&1; then
        pkill -f "bot_sec_manager" >/dev/null 2>&1 || true
        pkill -f "ClawdSecbot" >/dev/null 2>&1 || true
        log_info "Stopped running ClawdSecbot processes if present"
    else
        log_warn "pkill not found, skip process termination"
    fi
}

# 删除目标路径（文件或目录）。
remove_target_path() {
    local target="$1"
    [[ -e "$target" || -L "$target" ]] || return 0

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[dry-run] would remove: $target"
        return 0
    fi

    rm -rf "$target"
    log_info "Removed: $target"
}

# 执行删除动作。
execute_cleanup() {
    local count=0
    while IFS= read -r target; do
        [[ -n "${target// }" ]] || continue
        remove_target_path "$target"
        count=$((count + 1))
    done < <(printf '%s\n' "$DELETE_TARGETS")
    log_info "Cleanup finished, processed targets: $count"
}

# 显示确认提示。
confirm_if_needed() {
    if [[ "$FORCE" == "true" || "$DRY_RUN" == "true" ]]; then
        return 0
    fi

    echo "Detected platform: $TARGET_PLATFORM"
    echo "The script will remove ClawdSecbot generated files listed below:"
    printf '%s\n' "$DELETE_TARGETS"
    read -r -p "Continue cleanup? [y/N]: " answer
    case "$answer" in
        y|Y|yes|YES)
            ;;
        *)
            log_warn "Cleanup cancelled by user"
            exit 0
            ;;
    esac
}

# 校验系统级清理权限。
validate_permissions() {
    if [[ "$REMOVE_SYSTEM_FILES" != "true" ]]; then
        return 0
    fi
    if [[ "$(id -u)" -ne 0 ]]; then
        log_error "--remove-system-files requires root privileges"
        exit 1
    fi
}

# 脚本主流程。
main() {
    parse_args "$@"
    resolve_platform
    validate_permissions

    collect_common_targets
    if [[ "$TARGET_PLATFORM" == "macos" ]]; then
        collect_macos_targets
    else
        collect_linux_targets
    fi
    if [[ "$REMOVE_SYSTEM_FILES" == "true" ]]; then
        collect_system_targets
    fi

    stop_processes
    confirm_if_needed
    execute_cleanup
}

main "$@"
