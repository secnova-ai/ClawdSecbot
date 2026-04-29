#!/bin/bash

# 开启严格模式：命令失败、未定义变量、管道失败时立即退出。
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

PACKAGE_NAME="clawdsecbot"
APP_DISPLAY_NAME="ClawdSecbot"
DESKTOP_TARGET="desktop"
WEB_TARGET="web"
WEB_PACKAGE_NAME="${PACKAGE_NAME}-web"
WEB_EXECUTABLE_NAME="botsec_webd"
WEB_LAUNCHER_NAME="${PACKAGE_NAME}-web"
DEFAULT_VERSION="1.0.0"
DEFAULT_BUILD_NUMBER="$(date +"%Y%m%d%H%M")"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &> /dev/null && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
WORK_DIR="$PROJECT_ROOT/build/linux_packaging"
ROOTFS_DIR="$WORK_DIR/rootfs"
WEB_ROOTFS_DIR="$WORK_DIR/rootfs_web"
WEB_BUILD_DIR="$WORK_DIR/web_build"
SOURCE_ICON="$PROJECT_ROOT/scripts/icon_1024.png"
TRAY_ICON="$PROJECT_ROOT/images/tray_icon.png"
WEB_TAR_ARTIFACT=""

BUILD_DEB=false
BUILD_RPM=false
BUILD_MODE_EXPLICIT=false
VERSION="$DEFAULT_VERSION"
BUILD_NUMBER="$DEFAULT_BUILD_NUMBER"
BUILD_ARCH=""
PACKAGE_TYPE="community"
BRAND_NAME=""
DEB_ARCH=""
RPM_ARCH=""
FLUTTER_ARCH=""

# 打印帮助信息，说明参数与输出产物。
show_help() {
    cat << 'EOF'
Usage: ./scripts/build_linux_release.sh [OPTIONS]

Build Linux release artifacts for ClawdSecbot.
Default behavior builds desktop+web DEB/RPM plus web tar.gz installer package in one run.

Options:
  -v,  --version <X.Y.Z>     Semantic version (default: 1.0.0)
  -bn, --build <STAMP>       Build timestamp (default: current time, e.g. 202603230900)
       --build-number <STAMP>
  -ar, --arch <ARCH>         Target arch: x86_64|amd64|arm64
  -t,  --type <TYPE>         Package type: community|business (default: community)
  -br, --brand <NAME>        Brand suffix, only allowed when type=business
  --deb                      Build DEB package only
  --rpm                      Build RPM package only
  --all                      Build both DEB and RPM (default)
  -h, --help                 Show this help message and exit

Examples:
  ./scripts/build_linux_release.sh
  ./scripts/build_linux_release.sh -v 1.3.0 -bn 202603230900 -ar x86_64
  ./scripts/build_linux_release.sh --deb -v 1.3.0 -ar amd64
  ./scripts/build_linux_release.sh --rpm -v 1.3.0 -t business -br acme -ar arm64
EOF
}

# 打印错误日志并退出。
fail() {
    echo -e "${RED}ERROR: $1${NC}"
    exit 1
}

# 打印警告日志。
warn() {
    echo -e "${YELLOW}WARN: $1${NC}"
}

# 打印成功日志。
ok() {
    echo -e "${GREEN}$1${NC}"
}

# 打印普通日志，关键节点统一使用英文。
log_info() {
    echo "INFO: $1"
}

normalize_type() {
    local raw_type="$1"
    case "$raw_type" in
        personal|community)
            echo "community"
            ;;
        business)
            echo "business"
            ;;
        appstore)
            fail "Linux packages do not support type=appstore"
            ;;
        *)
            fail "Unsupported type: $raw_type"
            ;;
    esac
}

normalize_brand() {
    local raw_brand="$1"
    local normalized
    normalized="$(printf '%s' "$raw_brand" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//')"
    [[ -n "$normalized" ]] || fail "brand must contain letters or digits"
    echo "$normalized"
}

normalize_linux_arch() {
    local raw_arch="$1"
    case "$raw_arch" in
        x86_64|amd64)
            echo "x86_64"
            ;;
        arm64|aarch64)
            echo "arm64"
            ;;
        *)
            fail "Unsupported Linux arch: $raw_arch"
            ;;
    esac
}

artifact_type_segment() {
    echo "$PACKAGE_TYPE"
}

artifact_brand_segment() {
    if [[ "$PACKAGE_TYPE" == "business" && -n "$BRAND_NAME" ]]; then
        echo "-$BRAND_NAME"
    fi
}

build_artifact_name() {
    local target="$1"
    local extension="$2"
    printf '%s-%s-%s-%s-%s-%s%s.%s' \
        "$APP_DISPLAY_NAME" \
        "$target" \
        "$VERSION" \
        "$BUILD_NUMBER" \
        "$BUILD_ARCH" \
        "$(artifact_type_segment)" \
        "$(artifact_brand_segment)" \
        "$extension"
}

# 校验依赖命令是否可用。
require_command() {
    local cmd="$1"
    command -v "$cmd" >/dev/null 2>&1 || fail "Required command not found: $cmd"
}

# 解析脚本参数并确定构建模式与版本号。
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -v|--version)
                VERSION="${2:-}"
                shift 2
                ;;
            -bn|--build|--build-number)
                BUILD_NUMBER="${2:-}"
                shift 2
                ;;
            -ar|--arch)
                BUILD_ARCH="$(normalize_linux_arch "${2:-}")"
                shift 2
                ;;
            -t|--type)
                PACKAGE_TYPE="$(normalize_type "${2:-}")"
                shift 2
                ;;
            -br|--brand)
                BRAND_NAME="$(normalize_brand "${2:-}")"
                shift 2
                ;;
            --deb)
                BUILD_DEB=true
                BUILD_MODE_EXPLICIT=true
                shift
                ;;
            --rpm)
                BUILD_RPM=true
                BUILD_MODE_EXPLICIT=true
                shift
                ;;
            --all)
                BUILD_DEB=true
                BUILD_RPM=true
                BUILD_MODE_EXPLICIT=true
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            -*)
                fail "Unknown option: $1"
                ;;
        esac
    done

    if [[ "$BUILD_DEB" == false && "$BUILD_RPM" == false ]]; then
        BUILD_DEB=true
        BUILD_RPM=true
    fi

    if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        fail "Invalid version format: $VERSION (expect: X.Y.Z)"
    fi
    if ! [[ "$BUILD_NUMBER" =~ ^[0-9]+$ ]]; then
        fail "Invalid build number: $BUILD_NUMBER (expect: digits only)"
    fi
    if [[ "$PACKAGE_TYPE" != "business" && -n "$BRAND_NAME" ]]; then
        fail "brand is only allowed when type=business"
    fi
}

# 根据当前 CPU 架构推导 Flutter、DEB、RPM 所需架构名称。
detect_arch() {
    local host_arch
    local normalized_host_arch
    host_arch="$(uname -m)"
    normalized_host_arch="$(normalize_linux_arch "$host_arch")"

    if [[ -z "$BUILD_ARCH" ]]; then
        BUILD_ARCH="$normalized_host_arch"
    elif [[ "$BUILD_ARCH" != "$normalized_host_arch" ]]; then
        fail "Linux script does not support cross-build: requested $BUILD_ARCH but host is $normalized_host_arch"
    fi

    case "$BUILD_ARCH" in
        x86_64)
            DEB_ARCH="amd64"
            RPM_ARCH="x86_64"
            FLUTTER_ARCH="x64"
            ;;
        arm64)
            DEB_ARCH="arm64"
            RPM_ARCH="aarch64"
            FLUTTER_ARCH="arm64"
            ;;
    esac
}

# 更新 pubspec 版本号，保持构建产物版本一致。
update_pubspec_version() {
    log_info "Updating pubspec version to ${VERSION}+${BUILD_NUMBER}"
    sed -i "s/^version: .*/version: ${VERSION}+${BUILD_NUMBER}/" "$PROJECT_ROOT/pubspec.yaml"
    ok "Pubspec version updated"
}

# 构建 Linux preload 沙箱动态库并复制到用户策略目录。
build_sandbox_preload() {
    local sandbox_dir="$PROJECT_ROOT/go_lib/core/sandbox/linux_hook"
    local policy_dir="$HOME/.botsec/policies"
    local preload_so="$sandbox_dir/build/libsandbox_preload.so"

    if [[ ! -d "$sandbox_dir" ]]; then
        warn "sandbox directory not found at $sandbox_dir, skip preload build"
        return 0
    fi

    log_info "Building libsandbox_preload.so"
    mkdir -p "$sandbox_dir/build"
    cmake -S "$sandbox_dir" -B "$sandbox_dir/build" -DCMAKE_BUILD_TYPE=Release
    cmake --build "$sandbox_dir/build" --config Release

    if [[ -f "$preload_so" ]]; then
        mkdir -p "$policy_dir"
        cp "$preload_so" "$policy_dir/libsandbox_preload.so"
        ok "libsandbox_preload.so installed to $policy_dir/libsandbox_preload.so"
    else
        warn "libsandbox_preload.so not found after build"
    fi
}

# 执行公共构建流程，只构建一次供 DEB/RPM 复用。
build_release_bundle() {
    log_info "Running flutter clean"
    flutter clean

    log_info "Building Go shared library"
    bash "$PROJECT_ROOT/scripts/build_go.sh"
    build_sandbox_preload

    log_info "Resolving flutter dependencies"
    flutter pub get

    log_info "Building flutter linux release bundle"
    flutter build linux --release --no-tree-shake-icons

    log_info "Building flutter web release bundle"
    flutter build web \
        --target lib/main_web.dart \
        --pwa-strategy=none \
        --no-web-resources-cdn \
        --no-tree-shake-icons \
        --no-wasm-dry-run

    log_info "Building botsec_webd binary"
    rm -rf "$WEB_BUILD_DIR"
    mkdir -p "$WEB_BUILD_DIR/bin"
    (
        cd "$PROJECT_ROOT/go_lib"
        go build -buildvcs=false -o "$WEB_BUILD_DIR/bin/$WEB_EXECUTABLE_NAME" ./cmd/botsec_webd
    )

    ok "Desktop + Web release bundles built"
}

build_web_tarball_artifact() {
    local src_binary="$WEB_BUILD_DIR/bin/$WEB_EXECUTABLE_NAME"
    local src_web_dir="$PROJECT_ROOT/build/web"
    local src_preload_so="$PROJECT_ROOT/go_lib/core/sandbox/linux_hook/build/libsandbox_preload.so"
    local staging_root="$WORK_DIR/web_tar"
    local package_base
    local package_dir
    local output_file

    package_base="$(build_artifact_name "$WEB_TARGET" "tar.gz")"
    package_base="${package_base%.tar.gz}"
    package_dir="$staging_root/$package_base"
    output_file="$PROJECT_ROOT/build/${package_base}.tar.gz"

    [[ -f "$src_binary" ]] || fail "Web executable not found: $src_binary"
    [[ -d "$src_web_dir" ]] || fail "Flutter web bundle not found: $src_web_dir"
    [[ -f "$src_preload_so" ]] || fail "libsandbox_preload.so not found: $src_preload_so"

    rm -rf "$staging_root"
    mkdir -p "$package_dir/bin" "$package_dir/web" "$package_dir/lib"
    cp "$src_binary" "$package_dir/bin/$WEB_EXECUTABLE_NAME"
    chmod +x "$package_dir/bin/$WEB_EXECUTABLE_NAME"
    cp -a "$src_web_dir"/. "$package_dir/web/"
    cp "$src_preload_so" "$package_dir/lib/libsandbox_preload.so"

    cat > "$package_dir/install.sh" << EOF
#!/bin/bash
set -euo pipefail

if [[ "\${EUID}" -ne 0 ]]; then
  echo "Please run as root: sudo ./install.sh"
  exit 1
fi

BASE_DIR="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
INSTALL_LIB_DIR="/usr/lib/$WEB_PACKAGE_NAME"
INSTALL_BIN_LINK="/usr/bin/$WEB_LAUNCHER_NAME"
PRELOAD_TARGET="/usr/lib/$PACKAGE_NAME/libsandbox_preload.so"
PRELOAD_DIR="/usr/lib/$PACKAGE_NAME"

mkdir -p "\$INSTALL_LIB_DIR" "\$PRELOAD_DIR"
rm -rf "\$INSTALL_LIB_DIR"/web
mkdir -p "\$INSTALL_LIB_DIR"/web

install -m 0755 "\$BASE_DIR/bin/$WEB_EXECUTABLE_NAME" "\$INSTALL_LIB_DIR/$WEB_EXECUTABLE_NAME"
cp -a "\$BASE_DIR/web/." "\$INSTALL_LIB_DIR/web/"
install -m 0755 "\$BASE_DIR/lib/libsandbox_preload.so" "\$PRELOAD_TARGET"

cat > "\$INSTALL_BIN_LINK" << 'LAUNCHER'
#!/bin/bash
if [[ -z "\${BOTSEC_WEB_STATIC_DIR:-}" ]]; then
  export BOTSEC_WEB_STATIC_DIR="/usr/lib/$WEB_PACKAGE_NAME/web"
fi
exec /usr/lib/$WEB_PACKAGE_NAME/$WEB_EXECUTABLE_NAME "\$@"
LAUNCHER
chmod +x "\$INSTALL_BIN_LINK"

echo "Installed:"
echo "  - /usr/lib/$WEB_PACKAGE_NAME/$WEB_EXECUTABLE_NAME"
echo "  - /usr/lib/$WEB_PACKAGE_NAME/web"
echo "  - /usr/lib/$PACKAGE_NAME/libsandbox_preload.so"
echo "  - /usr/bin/$WEB_LAUNCHER_NAME"
echo ""
echo "Run with: $WEB_LAUNCHER_NAME --addr 0.0.0.0:18080"
EOF
    chmod +x "$package_dir/install.sh"

    rm -f "$output_file"
    tar -czf "$output_file" -C "$staging_root" "$package_base"
    WEB_TAR_ARTIFACT="$output_file"
    ok "Web tarball created: $output_file"
}

# 初始化 rootfs 目录结构。
prepare_rootfs_layout() {
    rm -rf "$ROOTFS_DIR"
    mkdir -p "$ROOTFS_DIR/usr/bin"
    mkdir -p "$ROOTFS_DIR/usr/lib/$PACKAGE_NAME"
    mkdir -p "$ROOTFS_DIR/usr/share/applications"
    mkdir -p "$ROOTFS_DIR/usr/share/pixmaps"
    for size in 16 22 24 32 48 64 128 256 512; do
        mkdir -p "$ROOTFS_DIR/usr/share/icons/hicolor/${size}x${size}/apps"
    done
}

# 将应用主程序、插件和必要动态库复制到 rootfs。
copy_application_files() {
    local bundle_dir="$PROJECT_ROOT/build/linux/$FLUTTER_ARCH/release/bundle"
    local plugins_dir="$PROJECT_ROOT/plugins"
    local preload_so="$PROJECT_ROOT/go_lib/core/sandbox/linux_hook/build/libsandbox_preload.so"
    local uninstall_script="$PROJECT_ROOT/scripts/uninstall/uninstall_unix.sh"

    [[ -d "$bundle_dir" ]] || fail "Flutter bundle not found: $bundle_dir"
    cp -a "$bundle_dir"/. "$ROOTFS_DIR/usr/lib/$PACKAGE_NAME/"

    if [[ -d "$plugins_dir" ]]; then
        mkdir -p "$ROOTFS_DIR/usr/lib/$PACKAGE_NAME/plugins"
        cp -a "$plugins_dir"/. "$ROOTFS_DIR/usr/lib/$PACKAGE_NAME/plugins/"
        ok "Plugins copied into package"
    else
        warn "plugins directory not found, skip plugins copy"
    fi

    if [[ -f "$preload_so" ]]; then
        cp "$preload_so" "$ROOTFS_DIR/usr/lib/$PACKAGE_NAME/libsandbox_preload.so"
        ok "libsandbox_preload.so copied into package"
    else
        warn "libsandbox_preload.so not found, package will not include preload sandbox library"
    fi

    cat > "$ROOTFS_DIR/usr/bin/$PACKAGE_NAME" << 'EOF'
#!/bin/bash
cd "$(eval echo ~$(whoami))"
exec /usr/lib/clawdsecbot/bot_sec_manager "$@" & > /dev/null
EOF
    chmod +x "$ROOTFS_DIR/usr/bin/$PACKAGE_NAME"

    # 将卸载脚本放到 bot_sec_manager 同级目录，便于用户直接执行。
    if [[ -f "$uninstall_script" ]]; then
        cp "$uninstall_script" "$ROOTFS_DIR/usr/lib/$PACKAGE_NAME/uninstall.sh"
        chmod +x "$ROOTFS_DIR/usr/lib/$PACKAGE_NAME/uninstall.sh"
        ok "Uninstall script copied to /usr/lib/$PACKAGE_NAME/uninstall.sh"
    else
        warn "uninstall script not found, skip copy: $uninstall_script"
    fi
}

# 生成多尺寸图标与桌面入口文件。
prepare_desktop_assets() {
    [[ -f "$SOURCE_ICON" ]] || fail "Source icon not found: $SOURCE_ICON"

    if command -v convert >/dev/null 2>&1; then
        for size in 16 22 24 32 48 64 128 256 512; do
            convert "$SOURCE_ICON" -resize "${size}x${size}" \
                "$ROOTFS_DIR/usr/share/icons/hicolor/${size}x${size}/apps/$PACKAGE_NAME.png"
        done
        ok "Icons generated with ImageMagick"
    else
        warn "ImageMagick convert not found, use fallback icons"
        for size_dir in 128x128 256x256 512x512; do
            cp "$SOURCE_ICON" "$ROOTFS_DIR/usr/share/icons/hicolor/$size_dir/apps/$PACKAGE_NAME.png"
        done
        if [[ -f "$TRAY_ICON" ]]; then
            for size_dir in 16x16 22x22 24x24 32x32 48x48 64x64; do
                cp "$TRAY_ICON" "$ROOTFS_DIR/usr/share/icons/hicolor/$size_dir/apps/$PACKAGE_NAME.png"
            done
        fi
    fi

    cp "$SOURCE_ICON" "$ROOTFS_DIR/usr/share/pixmaps/$PACKAGE_NAME.png"

    cat > "$ROOTFS_DIR/usr/share/applications/$PACKAGE_NAME.desktop" << EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=$APP_DISPLAY_NAME
Comment=AI Bot Security Manager
Exec=/usr/bin/$PACKAGE_NAME
Icon=$PACKAGE_NAME
Terminal=false
Categories=Utility;Security;
Keywords=security;bot;ai;manager;
StartupWMClass=com.clawdsecbot.guard
EOF
}

prepare_web_rootfs_layout() {
    rm -rf "$WEB_ROOTFS_DIR"
    mkdir -p "$WEB_ROOTFS_DIR/usr/bin"
    mkdir -p "$WEB_ROOTFS_DIR/usr/lib/$WEB_PACKAGE_NAME"
    mkdir -p "$WEB_ROOTFS_DIR/usr/lib/$WEB_PACKAGE_NAME/web"
    mkdir -p "$WEB_ROOTFS_DIR/usr/lib"
}

copy_web_application_files() {
    local web_bundle_dir="$PROJECT_ROOT/build/web"
    local web_binary="$WEB_BUILD_DIR/bin/$WEB_EXECUTABLE_NAME"
    local preload_so="$PROJECT_ROOT/go_lib/core/sandbox/linux_hook/build/libsandbox_preload.so"

    [[ -d "$web_bundle_dir" ]] || fail "Flutter web bundle not found: $web_bundle_dir"
    [[ -f "$web_binary" ]] || fail "Web binary not found: $web_binary"

    cp "$web_binary" "$WEB_ROOTFS_DIR/usr/lib/$WEB_PACKAGE_NAME/$WEB_EXECUTABLE_NAME"
    chmod +x "$WEB_ROOTFS_DIR/usr/lib/$WEB_PACKAGE_NAME/$WEB_EXECUTABLE_NAME"
    cp -a "$web_bundle_dir"/. "$WEB_ROOTFS_DIR/usr/lib/$WEB_PACKAGE_NAME/web/"

    if [[ -f "$preload_so" ]]; then
        cp "$preload_so" "$WEB_ROOTFS_DIR/usr/lib/libsandbox_preload.so"
        ok "Web package includes libsandbox_preload.so at /usr/lib/libsandbox_preload.so"
    else
        warn "libsandbox_preload.so not found, web package will skip preload sandbox library"
    fi

    cat > "$WEB_ROOTFS_DIR/usr/bin/$WEB_LAUNCHER_NAME" << EOF
#!/bin/bash
if [[ -z "\${BOTSEC_WEB_STATIC_DIR:-}" ]]; then
    export BOTSEC_WEB_STATIC_DIR="/usr/lib/$WEB_PACKAGE_NAME/web"
fi
exec /usr/lib/$WEB_PACKAGE_NAME/$WEB_EXECUTABLE_NAME "\$@"
EOF
    chmod +x "$WEB_ROOTFS_DIR/usr/bin/$WEB_LAUNCHER_NAME"
}

build_web_deb_package() {
    local deb_work="$WORK_DIR/deb_web/${WEB_PACKAGE_NAME}_${VERSION}_${DEB_ARCH}"
    local deb_file="$PROJECT_ROOT/build/$(build_artifact_name "$WEB_TARGET" "deb")"
    local installed_size

    log_info "Building Web DEB package"
    rm -rf "$deb_work"
    mkdir -p "$deb_work/DEBIAN"
    cp -a "$WEB_ROOTFS_DIR"/. "$deb_work/"

    installed_size="$(du -sk "$WEB_ROOTFS_DIR/usr" | cut -f1)"
    cat > "$deb_work/DEBIAN/control" << EOF
Package: $WEB_PACKAGE_NAME
Version: $VERSION
Section: utils
Priority: optional
Architecture: $DEB_ARCH
Installed-Size: $installed_size
Depends: libc6
Maintainer: ClawdSecbot Team <support@clawdsecbot.com>
Description: ClawdSecbot Web Bridge
 ClawdSecbot Web package includes the web bridge executable and Flutter Web static assets.
Homepage: https://github.com/clawdsecbot/bot_sec_manager
EOF

    rm -f "$deb_file"
    dpkg-deb --build --root-owner-group "$deb_work" "$deb_file"
    ok "Web DEB created: $deb_file"
}

build_web_rpm_package() {
    local rpm_root="$WORK_DIR/rpm_web/rpmbuild"
    local source_root="$WORK_DIR/rpm_web/${WEB_PACKAGE_NAME}-${VERSION}"
    local source_tar="$rpm_root/SOURCES/${WEB_PACKAGE_NAME}-${VERSION}.tar.gz"
    local spec_file="$rpm_root/SPECS/${WEB_PACKAGE_NAME}.spec"
    local rpm_out_dir="$rpm_root/RPMS/$RPM_ARCH"
    local rpm_candidates=()
    local rpm_input
    local rpm_file="$PROJECT_ROOT/build/$(build_artifact_name "$WEB_TARGET" "rpm")"

    log_info "Building Web RPM package"
    rm -rf "$WORK_DIR/rpm_web"
    mkdir -p "$rpm_root/BUILD" "$rpm_root/BUILDROOT" "$rpm_root/RPMS" "$rpm_root/SOURCES" "$rpm_root/SPECS" "$rpm_root/SRPMS"
    mkdir -p "$source_root"
    cp -a "$WEB_ROOTFS_DIR"/. "$source_root/"

    tar -czf "$source_tar" -C "$WORK_DIR/rpm_web" "${WEB_PACKAGE_NAME}-${VERSION}"

    cat > "$spec_file" << EOF
%global debug_package %{nil}
%global _debugsource_packages 0

Name:           $WEB_PACKAGE_NAME
Version:        $VERSION
Release:        $BUILD_NUMBER%{?dist}
Summary:        ClawdSecbot Web Bridge
License:        Proprietary
URL:            https://github.com/clawdsecbot/bot_sec_manager
BuildArch:      $RPM_ARCH
Source0:        %{name}-%{version}.tar.gz

%description
ClawdSecbot Web package includes botsec_webd and Flutter Web static files.

%prep
%setup -q

%build

%install
rm -rf %{buildroot}
mkdir -p %{buildroot}
cp -a * %{buildroot}/

%files
%defattr(-,root,root,-)
/usr/bin/$WEB_LAUNCHER_NAME
/usr/lib/$WEB_PACKAGE_NAME
EOF

    if [[ -f "$WEB_ROOTFS_DIR/usr/lib/libsandbox_preload.so" ]]; then
        cat >> "$spec_file" << EOF
/usr/lib/libsandbox_preload.so
EOF
    fi

    rpmbuild -bb --target "$RPM_ARCH" --define "_topdir $rpm_root" "$spec_file"

    shopt -s nullglob
    rpm_candidates=("$rpm_out_dir"/*.rpm)
    shopt -u nullglob
    [[ ${#rpm_candidates[@]} -gt 0 ]] || fail "Web RPM build finished but no rpm output found in $rpm_out_dir"
    rpm_input="${rpm_candidates[0]}"

    rm -f "$rpm_file"
    cp "$rpm_input" "$rpm_file"
    ok "Web RPM created: $rpm_file"
}

# 生成 DEB 控制文件并打包产出 deb 文件。
build_deb_package() {
    local deb_work="$WORK_DIR/deb/${PACKAGE_NAME}_${VERSION}_${DEB_ARCH}"
    local deb_file="$PROJECT_ROOT/build/$(build_artifact_name "$DESKTOP_TARGET" "deb")"
    local installed_size

    log_info "Building DEB package"
    rm -rf "$deb_work"
    mkdir -p "$deb_work/DEBIAN"
    cp -a "$ROOTFS_DIR"/. "$deb_work/"

    installed_size="$(du -sk "$ROOTFS_DIR/usr" | cut -f1)"
    cat > "$deb_work/DEBIAN/control" << EOF
Package: $PACKAGE_NAME
Version: $VERSION
Section: utils
Priority: optional
Architecture: $DEB_ARCH
Installed-Size: $installed_size
Depends: libgtk-3-0, libglib2.0-0, libayatana-appindicator3-1 | libappindicator3-1, libc6
Maintainer: ClawdSecbot Team <support@clawdsecbot.com>
Description: AI Bot Security Manager
 ClawdSecbot is a security management tool for AI bots.
 It provides protection, monitoring, and control features for AI bot interactions.
Homepage: https://github.com/clawdsecbot/bot_sec_manager
EOF

    cat > "$deb_work/DEBIAN/postinst" << 'EOF'
#!/bin/bash
set -e

# appindicator3 兼容：如果系统没有 libappindicator3.so.1 但有 libayatana-appindicator3.so.1，
# 则在 /usr/lib 创建符号链接，使 tray_manager 插件能正常加载。
COMPAT_LINK="/usr/lib/libappindicator3.so.1"
if ! ldconfig -p 2>/dev/null | grep -q "libappindicator3\.so\.1 "; then
    AYATANA_PATH=$(ldconfig -p 2>/dev/null | grep "libayatana-appindicator3\.so\.1 " | head -1 | sed 's/.*=> //')
    if [ -n "$AYATANA_PATH" ] && [ -f "$AYATANA_PATH" ]; then
        ln -sf "$AYATANA_PATH" "$COMPAT_LINK"
    fi
fi

if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -f -t /usr/share/icons/hicolor 2>/dev/null || true
fi
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q /usr/share/applications 2>/dev/null || true
fi
exit 0
EOF

    cat > "$deb_work/DEBIAN/postrm" << 'EOF'
#!/bin/bash
set -e
if [ "$1" = "remove" ] || [ "$1" = "purge" ]; then
    # 清理 appindicator3 兼容符号链接
    COMPAT_LINK="/usr/lib/libappindicator3.so.1"
    if [ -L "$COMPAT_LINK" ]; then
        rm -f "$COMPAT_LINK"
    fi

    if command -v gtk-update-icon-cache >/dev/null 2>&1; then
        gtk-update-icon-cache -f -t /usr/share/icons/hicolor 2>/dev/null || true
    fi
    if command -v update-desktop-database >/dev/null 2>&1; then
        update-desktop-database -q /usr/share/applications 2>/dev/null || true
    fi
fi
exit 0
EOF

    chmod +x "$deb_work/DEBIAN/postinst" "$deb_work/DEBIAN/postrm"

    rm -f "$deb_file"
    dpkg-deb --build --root-owner-group "$deb_work" "$deb_file"
    ok "DEB created: $deb_file"
}

# 生成 RPM spec 与源码归档并通过 rpmbuild 打包 rpm 文件。
build_rpm_package() {
    local rpm_root="$WORK_DIR/rpm/rpmbuild"
    local source_root="$WORK_DIR/rpm/${PACKAGE_NAME}-${VERSION}"
    local source_tar="$rpm_root/SOURCES/${PACKAGE_NAME}-${VERSION}.tar.gz"
    local spec_file="$rpm_root/SPECS/${PACKAGE_NAME}.spec"
    local rpm_out_dir="$rpm_root/RPMS/$RPM_ARCH"
    local rpm_candidates=()
    local rpm_input
    local rpm_file="$PROJECT_ROOT/build/$(build_artifact_name "$DESKTOP_TARGET" "rpm")"

    log_info "Building RPM package"
    rm -rf "$WORK_DIR/rpm"
    mkdir -p "$rpm_root/BUILD" "$rpm_root/BUILDROOT" "$rpm_root/RPMS" "$rpm_root/SOURCES" "$rpm_root/SPECS" "$rpm_root/SRPMS"
    mkdir -p "$source_root"
    cp -a "$ROOTFS_DIR"/. "$source_root/"

    tar -czf "$source_tar" -C "$WORK_DIR/rpm" "${PACKAGE_NAME}-${VERSION}"

    cat > "$spec_file" << EOF
%global debug_package %{nil}
%global _debugsource_packages 0

Name:           $PACKAGE_NAME
Version:        $VERSION
Release:        $BUILD_NUMBER%{?dist}
Summary:        AI Bot Security Manager
License:        Proprietary
URL:            https://github.com/clawdsecbot/bot_sec_manager
BuildArch:      $RPM_ARCH
Source0:        %{name}-%{version}.tar.gz
Provides:       libdesktop_multi_window_plugin.so()(64bit)
Provides:       libflutter_linux_gtk.so()(64bit)
Provides:       libscreen_retriever_linux_plugin.so()(64bit)
Provides:       libtray_manager_plugin.so()(64bit)
Provides:       liburl_launcher_linux_plugin.so()(64bit)
Provides:       libwindow_manager_plugin.so()(64bit)
Requires:       gtk3
Requires:       libappindicator3.so.1()(64bit)

%description
ClawdSecbot is a security management tool for AI bots.
It provides protection, monitoring, and control features for AI bot interactions.

%prep
%setup -q

%build

%install
rm -rf %{buildroot}
mkdir -p %{buildroot}
cp -a * %{buildroot}/

%post
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -f -t /usr/share/icons/hicolor >/dev/null 2>&1 || true
fi
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q /usr/share/applications >/dev/null 2>&1 || true
fi

%postun
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -f -t /usr/share/icons/hicolor >/dev/null 2>&1 || true
fi
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q /usr/share/applications >/dev/null 2>&1 || true
fi

%files
%defattr(-,root,root,-)
/usr/bin/$PACKAGE_NAME
/usr/lib/$PACKAGE_NAME
/usr/share/applications/$PACKAGE_NAME.desktop
/usr/share/icons/hicolor
/usr/share/pixmaps/$PACKAGE_NAME.png
EOF

    rpmbuild -bb --target "$RPM_ARCH" --define "_topdir $rpm_root" "$spec_file"

    shopt -s nullglob
    rpm_candidates=("$rpm_out_dir"/*.rpm)
    shopt -u nullglob
    [[ ${#rpm_candidates[@]} -gt 0 ]] || fail "RPM build finished but no rpm output found in $rpm_out_dir"
    rpm_input="${rpm_candidates[0]}"

    rm -f "$rpm_file"
    cp "$rpm_input" "$rpm_file"
    ok "RPM created: $rpm_file"
}

# 输出构建结果摘要，方便发布前核对。
print_summary() {
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    ok "Linux release build completed"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    if [[ -n "$WEB_TAR_ARTIFACT" ]]; then
        echo "Web TAR: $WEB_TAR_ARTIFACT"
    fi
    if [[ "$BUILD_DEB" == true ]]; then
        echo "Desktop DEB: $PROJECT_ROOT/build/$(build_artifact_name "$DESKTOP_TARGET" "deb")"
        echo "Web DEB:     $PROJECT_ROOT/build/$(build_artifact_name "$WEB_TARGET" "deb")"
    fi
    if [[ "$BUILD_RPM" == true ]]; then
        echo "Desktop RPM: $PROJECT_ROOT/build/$(build_artifact_name "$DESKTOP_TARGET" "rpm")"
        echo "Web RPM:     $PROJECT_ROOT/build/$(build_artifact_name "$WEB_TARGET" "rpm")"
    fi
}

main() {
    cd "$PROJECT_ROOT"
    parse_args "$@"
    detect_arch

    require_command flutter
    require_command cmake
    require_command sed
    require_command go
    require_command tar
    if [[ "$BUILD_DEB" == true ]]; then
        if command -v dpkg-deb >/dev/null 2>&1; then
            :
        elif [[ "$BUILD_RPM" == true && "$BUILD_MODE_EXPLICIT" == false ]]; then
            warn "dpkg-deb not found, fallback to RPM-only build on current host"
            BUILD_DEB=false
        else
            fail "Required command not found: dpkg-deb"
        fi
    fi
    if [[ "$BUILD_RPM" == true ]]; then
        require_command rpmbuild
    fi

    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Building Linux Release Packages"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Version: ${VERSION}+${BUILD_NUMBER}"
    echo "Package Type: $PACKAGE_TYPE"
    if [[ -n "$BRAND_NAME" ]]; then
        echo "Brand: $BRAND_NAME"
    fi
    echo "Architecture: deb=$DEB_ARCH rpm=$RPM_ARCH flutter=$FLUTTER_ARCH"
    echo "Build DEB: $BUILD_DEB"
    echo "Build RPM: $BUILD_RPM"
    echo "Targets: desktop + web"
    echo ""

    update_pubspec_version
    build_release_bundle
    build_web_tarball_artifact

    prepare_rootfs_layout
    copy_application_files
    prepare_desktop_assets
    prepare_web_rootfs_layout
    copy_web_application_files

    if [[ "$BUILD_DEB" == true ]]; then
        build_deb_package
        build_web_deb_package
    fi
    if [[ "$BUILD_RPM" == true ]]; then
        build_rpm_package
        build_web_rpm_package
    fi

    print_summary
}

main "$@"
