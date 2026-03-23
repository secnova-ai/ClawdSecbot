#!/bin/bash

# 遇到错误、未定义变量、管道错误时立即退出，保证发布链路失败快。
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 默认签名与上传配置，推荐通过环境变量提供。
# DEFAULT_APPSTORE_SIGN_IDENTITY=""
# DEFAULT_PERSONAL_SIGN_IDENTITY=""
# DEFAULT_DEV_SIGN_IDENTITY=""
# DEFAULT_APPSTORE_INSTALLER_IDENTITY=""
# DEFAULT_NOTARY_APPLE_ID=""
# DEFAULT_NOTARY_TEAM_ID=""
# DEFAULT_NOTARY_PASSWORD=""
# DEFAULT_APPLE_ID=""
# DEFAULT_ASC_PROVIDER=""
# DEFAULT_BUNDLE_ID=""
# DEFAULT_APP_SPECIFIC_PASSWORD=""

# 打印错误日志并退出。
fail() {
    echo -e "${RED}ERROR: $1${NC}"
    exit 1
}

# 打印告警日志。
warn() {
    echo -e "${YELLOW}WARN: $1${NC}"
}

# 打印成功日志。
ok() {
    echo -e "${GREEN}$1${NC}"
}

# 打印帮助信息，说明统一脚本的参数与示例。
show_help() {
    cat << 'EOF'
Usage:
  ./scripts/build_macos_release_new.sh --type <community|business|appstore> [options]

Description:
  Unified macOS release script for build/sign/package/notarize/upload.

Stages:
  all        完整流程: build->sign->package->[notarize|upload]
             appstore 自动上传, community/business 自动公证, 默认生成 Universal Binary (arm64 + x86_64).
  test       本地测试: build(debug)->sign->package, 跳过上传和公证, 生成当前机器单架构, 构建速度更快, 适合本地调试.
             appstore+test 使用 Apple Development 证书 + Developer profile, 生成可本地运行的沙箱化 .pkg.
  build      仅构建 Flutter app 和 Go 动态库.
  sign       仅签名 (需先完成 build).
  package    仅打包 DMG/PKG (需先完成 sign).
  notarize   仅公证 (仅 community/business, 需先完成 package 或传 --pkg-path).
  upload     仅上传 (仅 appstore, 需先完成 package 或传 --pkg-path).

Core Options:
  -t, --type <community|business|appstore>
                                       Release channel. `personal` is accepted as a legacy alias of `community`.
  --stage <all|test|build|sign|package|notarize|upload>
                                       Execution stage (default: all).
  -v, --version <X.Y.Z>                Version string (default: 1.0.0).
  -bn, --build <STAMP>                 Build timestamp (default: current time).
      --build-number <STAMP>
  -ar, --arch <universal|x86_64|arm64> Target arch (default: test=host, others=universal).
  -br, --brand <NAME>                  Brand suffix, only allowed when type=business.
  --work-dir <dir>                     Output root directory (default: build).
  --pkg-path <path>                    Package path for upload/notarize stage.

Notarization:
  --enable-notarization <true|false>   Default: community/business=true, appstore=false.
  --notary-keychain-profile <name>     Preferred credentials for notarytool.
  --notary-apple-id <email>            Fallback Apple ID for notarytool.
  --notary-team-id <team_id>           Fallback Team ID for notarytool.
  --notary-password <app_password>     Fallback app-specific password for notarytool.

Upload:
  --upload <true|false>                Used by upload stage only (default: false).
  --apple-id <email>                   Apple ID for altool upload.
  --asc-provider <team_id>             App Store Connect provider short name.
  --bundle-id <bundle_id>              Bundle identifier for upload logs.
  --app-specific-password <password>   App-specific password for altool.
  --provisioning-profile <path>        App Store provisioning profile path for app bundle.

Dev test:
  --dev-sign-identity <name>           Apple Development identity for test+appstore (default: auto).

Signing env overrides:
  PERSONAL_SIGN_IDENTITY               Override personal app sign identity.
  APPSTORE_SIGN_IDENTITY               Override appstore app sign identity.
  APPSTORE_INSTALLER_IDENTITY          Override appstore installer sign identity.
  DEV_SIGN_IDENTITY                    Override Apple Development identity for test+appstore.
  KEYCHAIN_PASSWORD                    Optional keychain unlock password for CI shell.

Examples:
  # ── 1. App Store Distribution ──

  # 一键全流程
  ./scripts/build_macos_release_new.sh --type appstore --stage all -v 1.2.3 -bn 202603230900

  # 分步执行
  ./scripts/build_macos_release_new.sh --type appstore --stage build -v 1.2.3 -bn 202603230900
  ./scripts/build_macos_release_new.sh --type appstore --stage sign -v 1.2.3 -bn 202603230900
  ./scripts/build_macos_release_new.sh --type appstore --stage package -v 1.2.3 -bn 202603230900
  ./scripts/build_macos_release_new.sh --type appstore --stage upload \
    --pkg-path build/build_macos_appstore/ClawdSecbot-1.2.3-202603230900-universal-appstore.pkg --upload true

  # ── 2. Community Distribution ──

  # 一键全流程
  ./scripts/build_macos_release_new.sh --type community --stage all -v 1.2.3 -bn 202603230900

  # 分步执行 (手动公证)
  ./scripts/build_macos_release_new.sh --type community --stage build -v 1.2.3 -bn 202603230900
  ./scripts/build_macos_release_new.sh --type community --stage sign -v 1.2.3 -bn 202603230900
  ./scripts/build_macos_release_new.sh --type community --stage package -v 1.2.3 -bn 202603230900
  ./scripts/build_macos_release_new.sh --type community --stage notarize \
    --pkg-path build/build_macos_community/ClawdSecbot-1.2.3-202603230900-universal-community.dmg -v 1.2.3 -bn 202603230900

  # ── 3. Local Test ──
  ./scripts/build_macos_release_new.sh --type community --stage test -v 1.2.3 -bn 202603230900 -ar arm64
  ./scripts/build_macos_release_new.sh --type appstore --stage test -v 1.2.3 -bn 202603230900 -ar x86_64
EOF
}

# 校验命令存在，避免执行到中途失败。
require_command() {
    local command_name="$1"
    command -v "$command_name" >/dev/null 2>&1 || fail "Required command not found: $command_name"
}

# 自增步骤并打印统一步骤日志。
STEP=0
next_step() {
    STEP=$((STEP + 1))
    echo "Step $STEP: $1"
}

default_build_number() {
    date +"%Y%m%d%H%M"
}

normalize_package_type() {
    local raw_type="$1"
    case "$raw_type" in
        personal|community)
            echo "community"
            ;;
        business)
            echo "business"
            ;;
        appstore)
            echo "appstore"
            ;;
        *)
            fail "Unsupported type: $raw_type"
            ;;
    esac
}

normalize_brand_name() {
    local raw_brand="$1"
    local normalized
    normalized="$(printf '%s' "$raw_brand" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//')"
    [[ -n "$normalized" ]] || fail "brand must contain letters or digits"
    echo "$normalized"
}

normalize_macos_arch() {
    local raw_arch="$1"
    case "$raw_arch" in
        universal|x86_64|arm64)
            echo "$raw_arch"
            ;;
        amd64)
            echo "x86_64"
            ;;
        *)
            fail "Unsupported macOS arch: $raw_arch"
            ;;
    esac
}

build_variant_for_flutter() {
    if [[ "$BUILD_TYPE" == "appstore" ]]; then
        echo "appstore"
    else
        echo "personal"
    fi
}

artifact_brand_segment() {
    if [[ "$BUILD_TYPE" == "business" && -n "$BRAND_NAME" ]]; then
        echo "-$BRAND_NAME"
    fi
}

build_package_basename() {
    printf '%s-%s-%s-%s-%s%s' \
        "ClawdSecbot" \
        "$VERSION" \
        "$BUILD_NUMBER" \
        "$BUILD_ARCH" \
        "$BUILD_TYPE" \
        "$(artifact_brand_segment)"
}

# 解析发布类型对应 entitlement 文件。
resolve_entitlements_file() {
    local build_type="$1"
    if [[ "$build_type" == "appstore" ]]; then
        echo "macos/Runner/AppStore.entitlements"
    else
        echo "macos/Runner/Personal.entitlements"
    fi
}

# 将 x86_64 映射为 Go 可识别架构名。
map_go_arch() {
    local arch="$1"
    if [[ "$arch" == "x86_64" ]]; then
        echo "amd64"
        return
    fi
    echo "$arch"
}

# 将当前机器架构标准化为 arm64/x86_64。
normalize_host_arch() {
    local raw_arch
    raw_arch="$(uname -m)"
    if [[ "$raw_arch" == "arm64" ]]; then
        echo "arm64"
    elif [[ "$raw_arch" == "x86_64" ]]; then
        echo "x86_64"
    else
        fail "Unsupported host architecture: $raw_arch"
    fi
}

# Flutter macOS 不支持 --target-platform 参数。
# --release 默认生成 Universal Binary, --debug 默认生成当前机器单架构。

# 从 keychain 中按关键字查找首个证书名称。
find_identity_by_keyword() {
    local keyword="$1"
    local identity_type="$2"
    security find-identity -v -p "$identity_type" | awk -F\" -v kw="$keyword" '$0 ~ kw {print $2; exit}'
}

# 按发布类型解析 app 签名证书，并做类型强校验。
# test + appstore 使用 Developer ID Application 证书，Gatekeeper 完全信任，支持沙箱。
resolve_app_sign_identity() {
    local build_type="$1"
    local identity=""
    # test + appstore: 使用 Developer ID Application 证书，Gatekeeper 信任且支持沙箱 entitlements。
    if [[ "$STAGE" == "test" && "$build_type" == "appstore" ]]; then
        identity="${DEV_SIGN_IDENTITY:-$DEFAULT_DEV_SIGN_IDENTITY}"
        if [[ -z "$identity" ]]; then
            identity="$(find_identity_by_keyword "Developer ID Application" "codesigning")"
        fi
        [[ -n "$identity" ]] || fail "Missing Developer ID Application identity for local appstore test"
        echo "$identity"
        return
    fi
    if [[ "$build_type" == "community" || "$build_type" == "business" ]]; then
        identity="${PERSONAL_SIGN_IDENTITY:-$DEFAULT_PERSONAL_SIGN_IDENTITY}"
        if [[ -z "$identity" ]]; then
            identity="$(find_identity_by_keyword "Developer ID Application" "codesigning")"
        fi
        [[ "$identity" == *"Developer ID Application"* ]] || fail "community/business requires Developer ID Application identity"
    else
        identity="${APPSTORE_SIGN_IDENTITY:-$DEFAULT_APPSTORE_SIGN_IDENTITY}"
        if [[ -z "$identity" ]]; then
            identity="$(find_identity_by_keyword "Apple Distribution" "codesigning")"
        fi
        [[ "$identity" == *"Apple Distribution"* ]] || fail "appstore requires Apple Distribution identity"
    fi
    [[ -n "$identity" ]] || fail "Missing app signing identity"
    echo "$identity"
}

# 解析 appstore pkg 签名证书，并做类型校验。
resolve_appstore_pkg_identity() {
    local identity="${APPSTORE_INSTALLER_IDENTITY:-$DEFAULT_APPSTORE_INSTALLER_IDENTITY}"
    if [[ -z "$identity" ]]; then
        identity="$(find_identity_by_keyword "3rd Party Mac Developer Installer" "basic")"
    fi
    if [[ -z "$identity" ]]; then
        identity="$(find_identity_by_keyword "Mac Installer Distribution" "basic")"
    fi
    [[ -n "$identity" ]] || fail "Missing installer signing identity for appstore"
    if [[ "$identity" != *"3rd Party Mac Developer Installer"* && "$identity" != *"Mac Installer Distribution"* ]]; then
        fail "appstore pkg requires installer identity"
    fi
    echo "$identity"
}

# 在 CI 场景下按需解锁 keychain，降低签名失败概率。
unlock_keychain_if_needed() {
    local keychain_path="${HOME}/Library/Keychains/login.keychain-db"
    if [[ -n "${KEYCHAIN_PASSWORD:-}" ]]; then
        echo "Unlocking keychain..."
        security unlock-keychain -p "${KEYCHAIN_PASSWORD}" "$keychain_path"
        security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "${KEYCHAIN_PASSWORD}" "$keychain_path" >/dev/null 2>&1 || true
        ok "Keychain unlocked"
    fi
}

thin_app_bundle_to_arch() {
    local app_bundle="$1"
    local target_arch="$2"
    local thinned_bundle="${app_bundle}.thinned"

    [[ "$target_arch" == "arm64" || "$target_arch" == "x86_64" ]] || fail "thin_app_bundle_to_arch only supports arm64/x86_64"
    rm -rf "$thinned_bundle"
    ditto --arch "$target_arch" "$app_bundle" "$thinned_bundle"
    rm -rf "$app_bundle"
    mv "$thinned_bundle" "$app_bundle"
}

# 使用当前 type 自动签名参数签署目标文件。
sign_file_auto() {
    local target_path="$1"
    shift || true
    local extra_args=("$@")
    local sign_args=(--force)
    if [[ "$APP_SIGN_IDENTITY" == *"Developer ID Application"* ]]; then
        sign_args+=(--timestamp --options runtime --sign "$APP_SIGN_IDENTITY")
    else
        sign_args+=(--sign "$APP_SIGN_IDENTITY")
    fi

    if [[ ${#extra_args[@]} -gt 0 ]]; then
        codesign "${sign_args[@]}" "${extra_args[@]}" "$target_path"
    else
        codesign "${sign_args[@]}" "$target_path"
    fi
}

# 判断目标文件是否为 Mach-O 二进制，避免依赖 rg。
is_macho_file() {
    local target_path="$1"
    file "$target_path" 2>/dev/null | grep -qE "Mach-O"
}

# 构建指定架构的 Go 动态库并复制到插件目录。
# 统一使用 botsec.dylib 作为产物名，与 build_go.sh 及 _rules 规范一致。
build_go_library_for_arch() {
    local target_arch="$1"
    local project_root="$2"
    local go_lib_dir="$project_root/go_lib"
    local plugins_dir="$project_root/plugins"
    local output_name="botsec"

    [[ -d "$go_lib_dir" ]] || fail "go_lib directory not found: $go_lib_dir"
    mkdir -p "$plugins_dir"

    # universal 需要分别构建 arm64/x86_64 并合并；单架构直接构建。
    local built_file="$go_lib_dir/${output_name}.dylib"
    if [[ "$target_arch" == "universal" ]]; then
        local tmp_arm="$go_lib_dir/${output_name}.arm64.dylib"
        local tmp_x86="$go_lib_dir/${output_name}.x86_64.dylib"
        echo "Building Go shared library for arm64..."
        (
            cd "$go_lib_dir"
            rm -f "${output_name}.dylib" "lib${output_name}.dylib" "${output_name}.h" "$tmp_arm" "$tmp_x86"
            CGO_ENABLED=1 GOOS=darwin GOARCH="arm64" go build -o "${output_name}.dylib" -buildmode=c-shared .
            if [[ -f "${output_name}.dylib" ]]; then
                mv "${output_name}.dylib" "$tmp_arm"
            elif [[ -f "lib${output_name}.dylib" ]]; then
                mv "lib${output_name}.dylib" "$tmp_arm"
            else
                fail "Go build output missing for arm64"
            fi
        )
        echo "Building Go shared library for x86_64..."
        (
            cd "$go_lib_dir"
            CGO_ENABLED=1 GOOS=darwin GOARCH="amd64" go build -o "${output_name}.dylib" -buildmode=c-shared .
            if [[ -f "${output_name}.dylib" ]]; then
                mv "${output_name}.dylib" "$tmp_x86"
            elif [[ -f "lib${output_name}.dylib" ]]; then
                mv "lib${output_name}.dylib" "$tmp_x86"
            else
                fail "Go build output missing for x86_64"
            fi
        )
        lipo -create "$tmp_arm" "$tmp_x86" -output "$plugins_dir/${output_name}.dylib"
        rm -f "$tmp_arm" "$tmp_x86"
    else
        local go_arch
        go_arch="$(map_go_arch "$target_arch")"
        echo "Building Go shared library for ${target_arch}..."
        (
            cd "$go_lib_dir"
            rm -f "${output_name}.dylib" "lib${output_name}.dylib" "${output_name}.h"
            CGO_ENABLED=1 GOOS=darwin GOARCH="$go_arch" go build -o "${output_name}.dylib" -buildmode=c-shared .
        )
        if [[ ! -f "$built_file" && -f "$go_lib_dir/lib${output_name}.dylib" ]]; then
            built_file="$go_lib_dir/lib${output_name}.dylib"
        fi
        [[ -f "$built_file" ]] || fail "Go build output missing for ${target_arch}"
        cp "$built_file" "$plugins_dir/${output_name}.dylib"
    fi
    if [[ -f "$go_lib_dir/${output_name}.h" ]]; then
        cp "$go_lib_dir/${output_name}.h" "$plugins_dir/${output_name}.h"
    elif [[ -f "$go_lib_dir/lib${output_name}.h" ]]; then
        cp "$go_lib_dir/lib${output_name}.h" "$plugins_dir/${output_name}.h"
    fi
    ok "Go shared library ready: $plugins_dir/${output_name}.dylib"
}

# 规范化 Framework 链接，避免 App Store 符号链接校验失败。
normalize_framework_symlinks() {
    local frameworks_root="$1"
    [[ -d "$frameworks_root" ]] || return 0

    # 创建或更新 framework 链接，确保链接目标符合预期。
    ensure_framework_symlink() {
        local link_path="$1"
        local expected_target="$2"
        if [[ -e "$link_path" && ! -L "$link_path" ]]; then
            rm -rf "$link_path"
        fi
        if [[ -L "$link_path" ]]; then
            local current_target
            current_target="$(readlink "$link_path" || true)"
            if [[ "$current_target" == "$expected_target" ]]; then
                return 0
            fi
            rm -f "$link_path"
        fi
        ln -s "$expected_target" "$link_path"
    }

    # 将 framework 统一整理为 Versions/A 结构，避免审核时报 malformed framework。
    normalize_single_framework_bundle() {
        local framework_path="$1"
        local framework_name="$2"
        local versions_dir="$framework_path/Versions"
        local version_dir="$versions_dir/A"
        mkdir -p "$version_dir"

        while IFS= read -r -d '' top_entry; do
            local entry_name
            entry_name="$(basename "$top_entry")"
            if [[ "$entry_name" == "Versions" ]]; then
                continue
            fi
            if [[ -L "$top_entry" ]]; then
                continue
            fi
            if [[ -e "$version_dir/$entry_name" ]]; then
                rm -rf "$version_dir/$entry_name"
            fi
            mv "$top_entry" "$version_dir/$entry_name"
        done < <(find "$framework_path" -mindepth 1 -maxdepth 1 -print0 2>/dev/null)

        ensure_framework_symlink "$versions_dir/Current" "A"
        ensure_framework_symlink "$framework_path/$framework_name" "Versions/Current/$framework_name"
        ensure_framework_symlink "$framework_path/Resources" "Versions/Current/Resources"
        if [[ -e "$version_dir/Info.plist" ]]; then
            ensure_framework_symlink "$framework_path/Info.plist" "Versions/Current/Info.plist"
        fi
    }

    while IFS= read -r -d '' framework; do
        local framework_name
        framework_name="$(basename "$framework" .framework)"
        normalize_single_framework_bundle "$framework" "$framework_name"
    done < <(find "$frameworks_root" -maxdepth 1 -name "*.framework" -type d -print0 2>/dev/null)
}

# 为 appstore 签名生成注入 application-identifier 和 team-identifier 的临时 entitlements 文件。
# 确保代码签名中的标识符与 provisioning profile 中的一致，否则 TestFlight 会拒绝上传。
prepare_appstore_entitlements() {
    local base_entitlements="$1"
    local team_id="$2"
    local bundle_id="$3"
    local app_identifier="${team_id}.${bundle_id}"
    local tmp_entitlements="${OUTPUT_DIR}/.entitlements_appstore_merged.plist"

    cp "$base_entitlements" "$tmp_entitlements"

    # 注入 com.apple.application-identifier (若尚未存在)。
    if ! /usr/libexec/PlistBuddy -c "Print :com.apple.application-identifier" "$tmp_entitlements" >/dev/null 2>&1; then
        /usr/libexec/PlistBuddy -c "Add :com.apple.application-identifier string $app_identifier" "$tmp_entitlements"
        echo "Injected com.apple.application-identifier = $app_identifier"
    else
        local existing_val
        existing_val="$(/usr/libexec/PlistBuddy -c "Print :com.apple.application-identifier" "$tmp_entitlements")"
        if [[ "$existing_val" != "$app_identifier" ]]; then
            /usr/libexec/PlistBuddy -c "Set :com.apple.application-identifier $app_identifier" "$tmp_entitlements"
            echo "Updated com.apple.application-identifier = $app_identifier (was $existing_val)"
        fi
    fi

    # 注入 com.apple.developer.team-identifier (若尚未存在)。
    if ! /usr/libexec/PlistBuddy -c "Print :com.apple.developer.team-identifier" "$tmp_entitlements" >/dev/null 2>&1; then
        /usr/libexec/PlistBuddy -c "Add :com.apple.developer.team-identifier string $team_id" "$tmp_entitlements"
        echo "Injected com.apple.developer.team-identifier = $team_id"
    else
        local existing_val
        existing_val="$(/usr/libexec/PlistBuddy -c "Print :com.apple.developer.team-identifier" "$tmp_entitlements")"
        if [[ "$existing_val" != "$team_id" ]]; then
            /usr/libexec/PlistBuddy -c "Set :com.apple.developer.team-identifier $team_id" "$tmp_entitlements"
            echo "Updated com.apple.developer.team-identifier = $team_id (was $existing_val)"
        fi
    fi

    echo "$tmp_entitlements"
}

# 为 test + appstore 生成本地可运行的 entitlements。
# 保留沙箱及网络/文件权限以模拟 App Store 环境，追加 Flutter debug 运行时权限。
# 移除 application-identifier 和 team-identifier 等受限 entitlement，
# 因为它们要求 provisioning profile 中的证书与签名证书一致(AMFI 校验)，
# 而本地 test 使用 Apple Development 证书，profile 不一定匹配。
prepare_test_appstore_entitlements() {
    local base_entitlements="$1"
    local tmp_entitlements="${OUTPUT_DIR}/.entitlements_appstore_merged.plist"

    cp "$base_entitlements" "$tmp_entitlements"

    # 删除受限 entitlement，避免 AMFI "No matching profile found" 拒绝启动。
    local restricted_keys=(
        "com.apple.application-identifier"
        "com.apple.developer.team-identifier"
    )
    for key in "${restricted_keys[@]}"; do
        if /usr/libexec/PlistBuddy -c "Print :${key}" "$tmp_entitlements" >/dev/null 2>&1; then
            /usr/libexec/PlistBuddy -c "Delete :${key}" "$tmp_entitlements"
            echo "Removed restricted entitlement for local test: ${key}" 1>&2
        fi
    done

    # 追加 Flutter debug 模式必需的运行时权限。
    local debug_keys=(
        "com.apple.security.cs.allow-jit"
        "com.apple.security.cs.allow-unsigned-executable-memory"
        "com.apple.security.cs.disable-library-validation"
        "com.apple.security.get-task-allow"
    )
    for key in "${debug_keys[@]}"; do
        if ! /usr/libexec/PlistBuddy -c "Print :${key}" "$tmp_entitlements" >/dev/null 2>&1; then
            /usr/libexec/PlistBuddy -c "Add :${key} bool true" "$tmp_entitlements"
            echo "Injected debug entitlement: ${key}" 1>&2
        fi
    done

    echo "$tmp_entitlements"
}

# 解析 appstore profile 路径，优先显式参数，其次回退 cert 目录默认文件。
# test + appstore 时优先使用 Developer profile 以便本地运行。
resolve_appstore_profile_path() {
    if [[ -n "${APPSTORE_PROVISIONING_PROFILE:-}" ]]; then
        echo "$APPSTORE_PROVISIONING_PROFILE"
        return
    fi
    local cert_dir="$PROJECT_ROOT/cert"
    # test 阶段优先使用 Developer profile，允许本地安装运行。
    if [[ "$STAGE" == "test" ]]; then
        local dev_profile="$cert_dir/ClawdSecbot_Developer.provisionprofile"
        if [[ -f "$dev_profile" ]]; then
            echo "$dev_profile"
            return
        fi
        warn "Developer provisioning profile not found, falling back to AppStore profile"
    fi
    local preferred_profile="$cert_dir/ClawdSecbot_AppStore.provisionprofile"
    if [[ -f "$preferred_profile" ]]; then
        echo "$preferred_profile"
        return
    fi
    local fallback_profile=""
    while IFS= read -r -d '' one_profile; do
        fallback_profile="$one_profile"
        break
    done < <(find "$cert_dir" -maxdepth 1 -name "*.provisionprofile" -type f -print0 2>/dev/null)
    [[ -n "$fallback_profile" ]] || fail "App Store provisioning profile not found in cert directory"
    echo "$fallback_profile"
}

# 向 app bundle 写入 embedded.provisionprofile，供 TestFlight 校验。
embed_appstore_profile_into_bundle() {
    local app_bundle="$1"
    [[ -d "$app_bundle" ]] || fail "App bundle missing: $app_bundle"
    local profile_path
    profile_path="$(resolve_appstore_profile_path)"
    [[ -f "$profile_path" ]] || fail "Provisioning profile not found: $profile_path"
    local embedded_profile="$app_bundle/Contents/embedded.provisionprofile"
    cp "$profile_path" "$embedded_profile"
    echo "Embedded provisioning profile: $embedded_profile"
}

# 深度优先签名 app 中所有 Mach-O 与 framework，再签名 app 外壳。
sign_app_bundle() {
    local app_bundle="$1"
    local entitlements_file="$2"
    # 清除 quarantine 等扩展属性，避免 App Store / TestFlight 上传被拒。
    xattr -cr "$app_bundle"
    echo "Cleared extended attributes from app bundle"
    if [[ -d "$app_bundle/Contents/Frameworks" ]]; then
        normalize_framework_symlinks "$app_bundle/Contents/Frameworks"
    fi
    local macho_files=()
    while IFS= read -r -d '' binary_path; do
        if is_macho_file "$binary_path"; then
            macho_files+=("$binary_path")
        fi
    done < <(find "$app_bundle" -type f -print0)

    local sorted_files=()
    while IFS= read -r sorted_path; do
        sorted_files+=("$sorted_path")
    done < <(for one_file in "${macho_files[@]}"; do
        local depth
        depth=$(printf '%s' "$one_file" | tr '/' '\n' | wc -l)
        printf '%d|%s\n' "$depth" "$one_file"
    done | sort -t'|' -k1 -rn | cut -d'|' -f2)

    for macho in "${sorted_files[@]}"; do
        sign_file_auto "$macho"
    done

    if [[ -d "$app_bundle/Contents/Frameworks" ]]; then
        while IFS= read -r -d '' framework_path; do
            sign_file_auto "$framework_path"
        done < <(find "$app_bundle/Contents/Frameworks" -maxdepth 1 -name "*.framework" -type d -print0 2>/dev/null)
    fi

    sign_file_auto "$app_bundle" --entitlements "$entitlements_file"
    codesign --verify --deep --strict --verbose=2 "$app_bundle"
    ok "App codesign verification passed"
}

# 扫描并校验 app 中所有 Mach-O 是否包含目标架构。
verify_app_arch_slices() {
    local app_bundle="$1"
    local expected_arch="$2"
    local checked_count=0
    while IFS= read -r -d '' binary_path; do
        if is_macho_file "$binary_path"; then
            local archs
            archs="$(lipo -archs "$binary_path" 2>/dev/null || true)"
            [[ "$archs" == *"$expected_arch"* ]] || fail "Missing arch slice ${expected_arch}: $binary_path"
            checked_count=$((checked_count + 1))
        fi
    done < <(find "$app_bundle" -type f -print0)
    echo "Verified Mach-O files: $checked_count"
}

# 在指定目录写入元数据，供 upload 阶段复用。
write_release_metadata() {
    local output_dir="$1"
    local app_bundle="$2"
    local package_path="$3"
    local package_name
    package_name="$(basename "$package_path")"
    cat > "$output_dir/release_meta.env" << EOF
TYPE=${BUILD_TYPE}
VERSION=${VERSION}
BUILD_NUMBER=${BUILD_NUMBER}
ARCH=${BUILD_ARCH}
BRAND=${BRAND_NAME}
APP_BUNDLE_REL=ClawdSecbot.app
PACKAGE_NAME=${package_name}
EOF
}

# 校验公证参数是否可用，避免提交阶段失败。
ensure_notary_credentials() {
    if [[ -n "${NOTARY_KEYCHAIN_PROFILE:-}" ]]; then
        return
    fi
    if [[ -n "${NOTARY_APPLE_ID:-}" && -n "${NOTARY_TEAM_ID:-}" && -n "${NOTARY_PASSWORD:-}" ]]; then
        return
    fi
    fail "Notary credentials missing"
}

# 提交制品到 Notary 并等待通过。
notarize_artifact() {
    local artifact_path="$1"
    echo "Submitting artifact to notarization..."
    if [[ -n "${NOTARY_KEYCHAIN_PROFILE:-}" ]]; then
        xcrun notarytool submit "$artifact_path" --keychain-profile "$NOTARY_KEYCHAIN_PROFILE" --wait
    else
        xcrun notarytool submit "$artifact_path" \
            --apple-id "$NOTARY_APPLE_ID" \
            --team-id "$NOTARY_TEAM_ID" \
            --password "$NOTARY_PASSWORD" \
            --wait
    fi
    xcrun stapler staple "$artifact_path"
    ok "Notarization and staple completed"
}

# 从目录中读取元数据文件并导出变量。
load_release_metadata() {
    local source_dir="$1"
    local meta_file="$source_dir/release_meta.env"
    [[ -f "$meta_file" ]] || fail "Metadata file not found: $meta_file"
    # shellcheck disable=SC1090
    source "$meta_file"
}

# 生成 personal 渠道 DMG 包。
create_personal_dmg() {
    local app_bundle="$1"
    local output_dir="$2"
    local dmg_dir="$output_dir/dmg_src"
    local package_path="$output_dir/$(build_package_basename).dmg"
    rm -rf "$dmg_dir"
    mkdir -p "$dmg_dir"
    cp -R "$app_bundle" "$dmg_dir/"
    (
        cd "$dmg_dir"
        ln -sf /Applications Applications
    )
    rm -f "$package_path"
    hdiutil create -volname "ClawdSecbot" -srcfolder "$dmg_dir" -ov -format UDZO -quiet "$package_path"

    # 对 DMG 进行签名
    codesign --timestamp --options runtime --sign "$APP_SIGN_IDENTITY" "$package_path"

    echo "$package_path"
}

# 生成 appstore 渠道 pkg 包。
# test 阶段生成未签名 pkg，跳过 installer 证书要求，仅用于本地安装验证。
create_appstore_pkg() {
    local app_bundle="$1"
    local output_dir="$2"
    local package_path="$output_dir/$(build_package_basename).pkg"
    rm -f "$package_path"
    # productbuild 会将进度日志输出到 stdout，这里重定向到 stderr，避免污染返回路径。
    if [[ "$STAGE" == "test" ]]; then
        productbuild --component "$app_bundle" /Applications "$package_path" 1>&2
        echo "PKG created without installer signing (test mode)" 1>&2
    else
        productbuild --component "$app_bundle" /Applications --sign "$PKG_SIGN_IDENTITY" "$package_path" 1>&2
    fi
    echo "$package_path"
}

# 上传 appstore 包到 App Store Connect。
upload_appstore_pkg() {
    local package_path="$1"
    [[ -f "$package_path" ]] || fail "Package not found for upload: $package_path"
    [[ "$BUILD_TYPE" == "appstore" ]] || fail "Upload is only allowed for appstore type"

    pkgutil --check-signature "$package_path"
    echo "Upload bundle id: $BUNDLE_ID"
    xcrun altool --validate-app -f "$package_path" -t macos -u "$APPLE_ID" -p "@env:APP_SPECIFIC_PASSWORD" --asc-provider "$ASC_PROVIDER"
    xcrun altool --upload-app -f "$package_path" -t macos -u "$APPLE_ID" -p "@env:APP_SPECIFIC_PASSWORD" --asc-provider "$ASC_PROVIDER"
    ok "Upload completed"
}

# 解析命令行参数并设置全局变量。
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -t|--type)
                BUILD_TYPE="$(normalize_package_type "${2:-}")"
                shift 2
                ;;
            --stage)
                STAGE="${2:-}"
                shift 2
                ;;
            -v|--version)
                VERSION="${2:-}"
                shift 2
                ;;
            -bn|--build|--build-number)
                BUILD_NUMBER="${2:-}"
                shift 2
                ;;
            -ar|--arch)
                BUILD_ARCH="$(normalize_macos_arch "${2:-}")"
                shift 2
                ;;
            -br|--brand)
                BRAND_NAME="$(normalize_brand_name "${2:-}")"
                shift 2
                ;;
            --work-dir)
                WORK_DIR="${2:-}"
                shift 2
                ;;
            --enable-notarization)
                ENABLE_NOTARIZATION="${2:-}"
                shift 2
                ;;
            --notary-keychain-profile)
                NOTARY_KEYCHAIN_PROFILE="${2:-}"
                shift 2
                ;;
            --notary-apple-id)
                NOTARY_APPLE_ID="${2:-}"
                shift 2
                ;;
            --notary-team-id)
                NOTARY_TEAM_ID="${2:-}"
                shift 2
                ;;
            --notary-password)
                NOTARY_PASSWORD="${2:-}"
                shift 2
                ;;
            --upload)
                UPLOAD_ENABLED="${2:-}"
                shift 2
                ;;
            --pkg-path)
                PACKAGE_PATH="${2:-}"
                shift 2
                ;;
            --apple-id)
                APPLE_ID="${2:-}"
                shift 2
                ;;
            --asc-provider)
                ASC_PROVIDER="${2:-}"
                shift 2
                ;;
            --bundle-id)
                BUNDLE_ID="${2:-}"
                shift 2
                ;;
            --app-specific-password)
                APP_SPECIFIC_PASSWORD="${2:-}"
                shift 2
                ;;
            --provisioning-profile)
                APPSTORE_PROVISIONING_PROFILE="${2:-}"
                shift 2
                ;;
            --dev-sign-identity)
                DEV_SIGN_IDENTITY="${2:-}"
                shift 2
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                fail "Unknown argument: $1"
                ;;
        esac
    done
}

# 校验参数组合与默认值，并初始化目录变量。
validate_and_init() {
    [[ "$BUILD_TYPE" == "community" || "$BUILD_TYPE" == "business" || "$BUILD_TYPE" == "appstore" ]] || fail "type must be community|business|appstore"
    [[ "$STAGE" == "all" || "$STAGE" == "test" || "$STAGE" == "build" || "$STAGE" == "sign" || "$STAGE" == "package" || "$STAGE" == "notarize" || "$STAGE" == "upload" ]] || fail "invalid stage"
    [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "version must be X.Y.Z"
    [[ "$BUILD_NUMBER" =~ ^[0-9]+$ ]] || fail "build-number must be digits only"
    if [[ "$BUILD_TYPE" != "business" && -n "$BRAND_NAME" ]]; then
        fail "brand is only allowed when type=business"
    fi

    if [[ -z "$BUILD_ARCH" ]]; then
        if [[ "$STAGE" == "test" ]]; then
            BUILD_ARCH="$(normalize_host_arch)"
        else
            BUILD_ARCH="universal"
        fi
    fi

    if [[ "$STAGE" == "test" && "$BUILD_ARCH" == "universal" ]]; then
        fail "test stage does not support universal arch"
    fi

    if [[ -z "${ENABLE_NOTARIZATION}" ]]; then
        if [[ "$BUILD_TYPE" == "appstore" ]]; then
            ENABLE_NOTARIZATION="false"
        else
            ENABLE_NOTARIZATION="true"
        fi
    fi

    if [[ "$BUILD_TYPE" == "appstore" && "$ENABLE_NOTARIZATION" == "true" ]]; then
        fail "appstore does not support notarization in this workflow"
    fi

    if [[ "$STAGE" == "upload" && "$BUILD_TYPE" != "appstore" ]]; then
        fail "upload stage only supports appstore type"
    fi

    RELEASE_ROOT="$PROJECT_ROOT/$WORK_DIR"
    OUTPUT_DIR="$RELEASE_ROOT/build_macos_${BUILD_TYPE}"
    mkdir -p "$OUTPUT_DIR"
}

# 构建 Flutter app 并复制到输出目录。
# test stage 使用 --debug (单架构, 当前机器), 其余 stage 使用 --release (Universal Binary)。
run_build_stage() {
    next_step "Start build stage"
    build_go_library_for_arch "$BUILD_ARCH" "$PROJECT_ROOT"
    flutter clean
    flutter pub get

    local flutter_build_mode
    local products_subdir
    if [[ "$STAGE" == "test" ]]; then
        flutter_build_mode="--debug"
        products_subdir="Debug"
        echo "Building Flutter (debug, single-arch: $BUILD_ARCH)"
    else
        flutter_build_mode="--release"
        products_subdir="Release"
        echo "Building Flutter (release, arch: $BUILD_ARCH)"
    fi

    # Flutter macOS 不支持 --target-platform, --release 默认 Universal, --debug 默认当前机器架构。
    flutter build macos \
        $flutter_build_mode \
        --no-tree-shake-icons \
        --dart-define=BUILD_VARIANT="$(build_variant_for_flutter)" \
        --build-name="$VERSION" \
        --build-number="$BUILD_NUMBER"

    APP_BUNDLE="$PROJECT_ROOT/build/macos/Build/Products/${products_subdir}/ClawdSecbot.app"
    [[ -d "$APP_BUNDLE" ]] || fail "App bundle not found: $APP_BUNDLE"
    mkdir -p "$APP_BUNDLE/Contents/Resources/plugins"
    for f in "$PROJECT_ROOT/plugins/"*.dylib "$PROJECT_ROOT/plugins/"*.h; do
        [[ -f "$f" ]] && cp "$f" "$APP_BUNDLE/Contents/Resources/plugins/"
    done
    normalize_framework_symlinks "$APP_BUNDLE/Contents/Frameworks"

    mkdir -p "$OUTPUT_DIR"
    rm -rf "$OUTPUT_DIR/ClawdSecbot.app"
    cp -R "$APP_BUNDLE" "$OUTPUT_DIR/ClawdSecbot.app"
    APP_BUNDLE="$OUTPUT_DIR/ClawdSecbot.app"

    if [[ "$BUILD_ARCH" != "universal" && "$STAGE" != "test" ]]; then
        thin_app_bundle_to_arch "$APP_BUNDLE" "$BUILD_ARCH"
    fi

    if [[ "$BUILD_ARCH" == "universal" ]]; then
        verify_app_arch_slices "$APP_BUNDLE" "arm64"
        verify_app_arch_slices "$APP_BUNDLE" "x86_64"
    else
        verify_app_arch_slices "$APP_BUNDLE" "$BUILD_ARCH"
    fi
    ok "Build stage completed"
}

# 执行签名阶段，对 app 进行自动证书签名。
run_sign_stage() {
    next_step "Start sign stage"
    [[ -d "$OUTPUT_DIR/ClawdSecbot.app" ]] || fail "App bundle missing for sign stage: $OUTPUT_DIR/ClawdSecbot.app"
    APP_BUNDLE="$OUTPUT_DIR/ClawdSecbot.app"
    local sign_entitlements="$ENTITLEMENTS_ABS"
    if [[ "$BUILD_TYPE" == "appstore" ]]; then
        if [[ "$STAGE" == "test" ]]; then
            # test + appstore: Developer ID 签名无需 provisioning profile，嵌入反而导致 Code Signing 报错。
            echo "Skipping provisioning profile embedding (Developer ID test mode)"
            sign_entitlements="$(prepare_test_appstore_entitlements "$ENTITLEMENTS_ABS")"
            echo "Using test entitlements (sandbox + debug): $sign_entitlements"
        else
            embed_appstore_profile_into_bundle "$APP_BUNDLE"
            # 动态注入 application-identifier 和 team-identifier，确保与 provisioning profile 一致。
            sign_entitlements="$(prepare_appstore_entitlements "$ENTITLEMENTS_ABS" "$ASC_PROVIDER" "$BUNDLE_ID")"
            echo "Using merged entitlements: $sign_entitlements"
        fi
    fi
    sign_app_bundle "$APP_BUNDLE" "$sign_entitlements"
    ok "Sign stage completed"
}

# 执行打包阶段，按 type 输出 DMG 或 PKG。
run_package_stage() {
    next_step "Start package stage"
    [[ -d "$OUTPUT_DIR/ClawdSecbot.app" ]] || fail "App bundle missing for package stage: $OUTPUT_DIR/ClawdSecbot.app"
    APP_BUNDLE="$OUTPUT_DIR/ClawdSecbot.app"
    # test 模式使用 Developer ID 签名，不嵌入 provisioning profile；非 test 模式必须有。
    if [[ "$BUILD_TYPE" == "appstore" && "$STAGE" != "test" && ! -f "$APP_BUNDLE/Contents/embedded.provisionprofile" ]]; then
        fail "Missing embedded.provisionprofile in app bundle, run sign stage first"
    fi
    if [[ "$BUILD_TYPE" == "community" || "$BUILD_TYPE" == "business" ]]; then
        PACKAGE_PATH="$(create_personal_dmg "$APP_BUNDLE" "$OUTPUT_DIR")"
    else
        PACKAGE_PATH="$(create_appstore_pkg "$APP_BUNDLE" "$OUTPUT_DIR")"
    fi
    # 仅取最后一行路径，规避第三方命令日志混入变量。
    PACKAGE_PATH="$(printf '%s\n' "$PACKAGE_PATH" | awk 'NF{line=$0} END{print line}')"
    [[ -f "$PACKAGE_PATH" ]] || fail "Package build output is invalid: $PACKAGE_PATH"
    write_release_metadata "$OUTPUT_DIR" "$APP_BUNDLE" "$PACKAGE_PATH"
    ok "Package stage completed: $PACKAGE_PATH"
}

# 执行公证阶段，仅 personal 支持。
run_notarize_stage() {
    next_step "Start notarize stage"
    [[ "$BUILD_TYPE" != "appstore" ]] || fail "notarize stage is only supported for community/business types"
    [[ "$ENABLE_NOTARIZATION" == "true" ]] || fail "notarization is disabled"
    if [[ -z "$PACKAGE_PATH" ]]; then
        [[ -f "$OUTPUT_DIR/release_meta.env" ]] || fail "No package metadata found for notarize stage"
        load_release_metadata "$OUTPUT_DIR"
    fi
    ensure_notary_credentials
    notarize_artifact "$PACKAGE_PATH"
    ok "Notarize stage completed"
}

# 执行上传阶段，仅 appstore 支持。
run_upload_stage() {
    next_step "Start upload stage"
    [[ "$BUILD_TYPE" == "appstore" ]] || fail "upload stage only supports appstore type"
    [[ "$UPLOAD_ENABLED" == "true" ]] || fail "upload requires --upload true"

    if [[ -z "$PACKAGE_PATH" ]]; then
        if [[ -f "$OUTPUT_DIR/release_meta.env" ]]; then
            load_release_metadata "$OUTPUT_DIR"
            PACKAGE_PATH="$OUTPUT_DIR/${PACKAGE_NAME:-}"
        else
            fail "No package metadata found for upload stage, pass --pkg-path"
        fi
    fi

    [[ -n "$APP_SPECIFIC_PASSWORD" ]] || fail "app-specific-password is required for upload"
    export APP_SPECIFIC_PASSWORD
    upload_appstore_pkg "$PACKAGE_PATH"
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &> /dev/null && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

BUILD_TYPE="community"
STAGE="all"
WORK_DIR="build"
BUILD_ARCH=""
PACKAGE_PATH=""
UPLOAD_ENABLED="false"
RELEASE_ROOT=""
OUTPUT_DIR=""
APP_BUNDLE=""
ENTITLEMENTS_ABS=""
APP_SIGN_IDENTITY=""
PKG_SIGN_IDENTITY=""

VERSION="1.0.0"
BUILD_NUMBER="$(default_build_number)"
BRAND_NAME=""

ENABLE_NOTARIZATION=""
NOTARY_KEYCHAIN_PROFILE="${NOTARY_KEYCHAIN_PROFILE:-}"
NOTARY_APPLE_ID="${NOTARY_APPLE_ID:-$DEFAULT_NOTARY_APPLE_ID}"
NOTARY_TEAM_ID="${NOTARY_TEAM_ID:-$DEFAULT_NOTARY_TEAM_ID}"
NOTARY_PASSWORD="${NOTARY_PASSWORD:-$DEFAULT_NOTARY_PASSWORD}"
APPLE_ID="${APPLE_ID:-$DEFAULT_APPLE_ID}"
ASC_PROVIDER="${ASC_PROVIDER:-$DEFAULT_ASC_PROVIDER}"
BUNDLE_ID="${BUNDLE_ID:-$DEFAULT_BUNDLE_ID}"
APP_SPECIFIC_PASSWORD="${APP_SPECIFIC_PASSWORD:-$DEFAULT_APP_SPECIFIC_PASSWORD}"
DEV_SIGN_IDENTITY="${DEV_SIGN_IDENTITY:-$DEFAULT_DEV_SIGN_IDENTITY}"
APPSTORE_PROVISIONING_PROFILE="${APPSTORE_PROVISIONING_PROFILE:-}"

parse_args "$@"
validate_and_init

ENTITLEMENTS_REL="$(resolve_entitlements_file "$BUILD_TYPE")"
ENTITLEMENTS_ABS="$PROJECT_ROOT/$ENTITLEMENTS_REL"
[[ -f "$ENTITLEMENTS_ABS" ]] || fail "Entitlements file not found: $ENTITLEMENTS_ABS"

require_command flutter
require_command go
require_command codesign
require_command security
require_command xcrun
require_command hdiutil
require_command lipo
require_command file
if [[ "$BUILD_TYPE" == "appstore" ]]; then
    require_command productbuild
    require_command pkgutil
fi

APP_SIGN_IDENTITY="$(resolve_app_sign_identity "$BUILD_TYPE")"
# test + appstore 不需要 installer 签名证书，pkg 以未签名形式供本地安装。
if [[ "$BUILD_TYPE" == "appstore" && "$STAGE" != "test" ]]; then
    PKG_SIGN_IDENTITY="$(resolve_appstore_pkg_identity)"
fi
unlock_keychain_if_needed

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Start macOS release workflow"
echo "Type: $BUILD_TYPE"
echo "Stage: $STAGE"
echo "Arch: $BUILD_ARCH"
echo "Version: $VERSION+$BUILD_NUMBER"
if [[ -n "$BRAND_NAME" ]]; then
    echo "Brand: $BRAND_NAME"
fi
echo "Output: $OUTPUT_DIR"
echo "App sign identity: $APP_SIGN_IDENTITY"
if [[ "$BUILD_TYPE" == "appstore" && "$STAGE" != "test" ]]; then
    echo "Pkg sign identity: $PKG_SIGN_IDENTITY"
elif [[ "$BUILD_TYPE" == "appstore" && "$STAGE" == "test" ]]; then
    echo "Pkg sign identity: (unsigned, test mode)"
fi
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [[ "$STAGE" == "all" ]]; then
    run_build_stage
    run_sign_stage
    run_package_stage
    if [[ "$BUILD_TYPE" != "appstore" && "$ENABLE_NOTARIZATION" == "true" ]]; then
        run_notarize_stage
    fi
    # appstore 全流程在 all 模式下自动上传，避免再次手动触发 upload 阶段。
    if [[ "$BUILD_TYPE" == "appstore" ]]; then
        UPLOAD_ENABLED="true"
        run_upload_stage
    fi
elif [[ "$STAGE" == "test" ]]; then
    # 仅构建+签名+打包，跳过公证和上传，用于本地测试验证。
    run_build_stage
    run_sign_stage
    run_package_stage
    ok "Test build completed (no upload/notarize)"
elif [[ "$STAGE" == "build" ]]; then
    run_build_stage
elif [[ "$STAGE" == "sign" ]]; then
    run_sign_stage
elif [[ "$STAGE" == "package" ]]; then
    run_package_stage
elif [[ "$STAGE" == "notarize" ]]; then
    run_notarize_stage
elif [[ "$STAGE" == "upload" ]]; then
    run_upload_stage
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
ok "Workflow completed successfully"
if [[ -n "$PACKAGE_PATH" ]]; then
    echo "Package: $PACKAGE_PATH"
fi
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
