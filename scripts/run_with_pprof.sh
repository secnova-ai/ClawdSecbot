#!/bin/bash
# run_with_pprof.sh — 构建并以 pprof 性能分析模式运行应用
#
# 用法:
#   ./scripts/run_with_pprof.sh                         # 使用默认端口 6060, community
#   ./scripts/run_with_pprof.sh 9090                    # 使用自定义端口 9090
#   ./scripts/run_with_pprof.sh 6060 business           # 使用 business 类型
#   BOTSEC_PPROF_PORT=8080 ./scripts/run_with_pprof.sh  # 通过环境变量指定端口
#
# Linux: 在启动 Flutter 前会 cmake 重新编译 go_lib/core/sandbox/linux_hook/preload.c，并将 libsandbox_preload.so
# 覆盖复制到 ~/.botsec/policies/，与 scripts/build_linux_release.sh 中 build_sandbox_preload 一致。
#
# pprof 常用命令:
#   go tool pprof http://127.0.0.1:6060/debug/pprof/heap          # 堆内存分析
#   go tool pprof http://127.0.0.1:6060/debug/pprof/profile?seconds=30  # CPU 分析 (30秒采样)
#   go tool pprof http://127.0.0.1:6060/debug/pprof/goroutine     # goroutine 分析
#   go tool pprof http://127.0.0.1:6060/debug/pprof/allocs        # 内存分配分析
#   go tool pprof http://127.0.0.1:6060/debug/pprof/block         # 阻塞分析
#   go tool pprof http://127.0.0.1:6060/debug/pprof/mutex         # 互斥锁竞争分析
#
# 浏览器查看:
#   open http://127.0.0.1:6060/debug/pprof/
#
set -e

PROJECT_ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." &> /dev/null && pwd )"
cd "$PROJECT_ROOT"

# 确定 pprof 端口: 命令行参数 > 环境变量 > 默认值 6060
PPROF_PORT="${1:-${BOTSEC_PPROF_PORT:-6060}}"
BUILD_TYPE="${2:-community}"

if [[ "$BUILD_TYPE" != "community" && "$BUILD_TYPE" != "business" ]]; then
    echo "错误: BUILD_TYPE 仅支持 community 或 business"
    exit 1
fi

# sudo 执行助手: 优先无交互执行(已有缓存凭据), 否则回退到交互式 sudo
run_with_sudo() {
    if sudo -n true 2>/dev/null; then
        sudo -n "$@"
    else
        sudo "$@"
    fi
}

echo "============================================"
echo "  BotSecManager — pprof 性能分析模式"
echo "============================================"
echo "Type: $BUILD_TYPE"
echo ""

# Step 1: 构建 Go 插件
echo "[1/3] 构建 Go 插件..."
"$PROJECT_ROOT/scripts/build_openclaw_plugin.sh"
echo ""

# Step 2 (Linux only): 重新编译 LD_PRELOAD 沙箱库并复制到策略目录，与 gateway 查找路径一致
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "[2/3] 构建并安装 libsandbox_preload.so..."
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
            echo "  已安装: $POLICY_DIR/libsandbox_preload.so"

            # 同步替换系统路径的预加载库, 供网关注入时直接使用
            run_with_sudo mkdir -p "$SYSTEM_LIB_DIR"
            run_with_sudo install -m 0755 "$PRELOAD_SO" "$SYSTEM_PRELOAD_SO"
            echo "  已替换: $SYSTEM_PRELOAD_SO"
        else
            echo "  警告: 构建后未找到 $PRELOAD_SO，沙箱 LD_PRELOAD 可能仍为旧版本"
        fi
    else
        echo "  警告: 未找到 $SANDBOX_DIR，跳过沙箱库构建"
    fi
    echo ""
else
    echo "[2/3] 非 Linux，跳过 libsandbox_preload.so 构建"
    echo ""
fi

# Step 3: 检测操作系统并启动 Flutter 应用（带 pprof）
echo "[3/3] 启动 Flutter 应用（pprof 端口: $PPROF_PORT）..."
echo ""
echo "  pprof 地址: http://127.0.0.1:${PPROF_PORT}/debug/pprof/"
echo ""
echo "  常用分析命令:"
echo "    go tool pprof http://127.0.0.1:${PPROF_PORT}/debug/pprof/heap"
echo "    go tool pprof http://127.0.0.1:${PPROF_PORT}/debug/pprof/profile?seconds=30"
echo "    go tool pprof http://127.0.0.1:${PPROF_PORT}/debug/pprof/goroutine"
echo ""
echo "============================================"
echo ""

# 自动检测操作系统
if [[ "$OSTYPE" == "darwin"* ]]; then
    FLUTTER_DEVICE="macos"
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    FLUTTER_DEVICE="linux"
elif [[ "$OSTYPE" == "msys" || "$OSTYPE" == "win32" ]]; then
    FLUTTER_DEVICE="windows"
else
    echo "警告: 未能自动识别操作系统 ($OSTYPE)，尝试使用 linux"
    FLUTTER_DEVICE="linux"
fi

echo "检测到操作系统: $FLUTTER_DEVICE"
echo ""

# Linux 开发模式: 安装 .desktop 文件和多尺寸图标，使 GNOME 能正确显示应用名、
# 窗口图标和托盘图标。托盘需要小尺寸（22-48px），窗口/Dock 需要大尺寸。
if [[ "$FLUTTER_DEVICE" == "linux" ]]; then
    DESKTOP_DIR="$HOME/.local/share/applications"
    ICON_BASE="$HOME/.local/share/icons/hicolor"
    DESKTOP_FILE="$DESKTOP_DIR/com.clawdsecbot.guard.desktop"
    SOURCE_ICON="$PROJECT_ROOT/scripts/icon_1024.png"
    TRAY_ICON="$PROJECT_ROOT/images/tray_icon.png"

    if [ -f "$SOURCE_ICON" ]; then
        ICON_SIZES=(16 22 24 32 48 64 128 256)
        for SIZE in "${ICON_SIZES[@]}"; do
            mkdir -p "$ICON_BASE/${SIZE}x${SIZE}/apps"
        done

        if command -v convert &> /dev/null; then
            for SIZE in "${ICON_SIZES[@]}"; do
                convert "$SOURCE_ICON" -resize "${SIZE}x${SIZE}" \
                    "$ICON_BASE/${SIZE}x${SIZE}/apps/clawdsecbot.png"
            done
        else
            # 无 ImageMagick 时: 大尺寸用源图标，小尺寸用 64x64 托盘图标
            for SIZE in 128 256; do
                cp "$SOURCE_ICON" "$ICON_BASE/${SIZE}x${SIZE}/apps/clawdsecbot.png"
            done
            if [ -f "$TRAY_ICON" ]; then
                for SIZE in 16 22 24 32 48 64; do
                    cp "$TRAY_ICON" "$ICON_BASE/${SIZE}x${SIZE}/apps/clawdsecbot.png"
                done
            fi
        fi

        # 更新图标缓存，使 GNOME/GTK 立即识别新图标
        gtk-update-icon-cache -f -t "$ICON_BASE" 2>/dev/null || true

        mkdir -p "$DESKTOP_DIR"
        cat > "$DESKTOP_FILE" << DESKTOP_EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=ClawdSecbot
Comment=AI Bot Security Manager (Dev)
Icon=clawdsecbot
Terminal=false
Categories=Utility;Security;
StartupWMClass=com.clawdsecbot.guard
NoDisplay=true
DESKTOP_EOF
        echo "已安装开发用 .desktop 文件和多尺寸图标"
    fi
fi

export BOTSEC_PPROF_PORT="$PPROF_PORT"
# 禁用 Flutter 自动更新检查，避免网络问题
export FLUTTER_STORAGE_BASE_URL="https://storage.flutter-io.cn"
export PUB_HOSTED_URL="https://pub.flutter-io.cn"
export FLUTTER_GIT_URL="https://gitee.com/mirrors/flutter.git"
# 跳过 git fetch 操作
export FLUTTER_ALREADY_LOCKED=true

# 首次运行或缓存被清理时，必须先生成 package_config 和插件符号链接
if [ ! -f "$PROJECT_ROOT/.dart_tool/package_config.json" ]; then
    echo "Bootstrap Flutter dependencies: package_config.json is missing."
    flutter pub get
fi

exec flutter run -d "$FLUTTER_DEVICE" --no-pub \
    --dart-define=BUILD_VARIANT=personal \
    --dart-define=BUILD_TYPE="$BUILD_TYPE"
