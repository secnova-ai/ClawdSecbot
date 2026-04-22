# Scripts Guide / 脚本说明

本目录存放 ClawdSecbot 的构建、打包、图标生成和调试脚本。

旧版 README 中提到的 `build_release.sh`、`run.sh` 和 Docker 发布流程已经不再适用于当前仓库。当前以 Flutter 桌面端、Go 动态库插件，以及各平台原生打包脚本为主。

## Quick Start / 快速入口

按用途选择脚本：

- 构建共享库并同步到 `plugins/`：`./scripts/build_go.sh`
- 构建 Linux 发布包：`./scripts/build_linux_release.sh`
- 构建 macOS 发布包：`./scripts/build_macos_release_new.sh`
- 构建 Windows 发布包：`.\scripts\build_windows_release.ps1`
- 生成应用和托盘图标：`./scripts/generate_icons.sh`
- 以 pprof 模式运行应用：
  - macOS/Linux: `./scripts/run_with_pprof.sh`
  - Windows: `.\scripts\run_with_pprof.ps1`

## Script List / 脚本列表

### `build_go.sh`

用途：
- 构建 `go_lib/` 共享库并复制到 `plugins/`。
- 额外会清理 `go_lib/` 中的旧头文件与旧动态库文件。

用法：

```bash
./scripts/build_go.sh
```

说明：
- 该脚本是本仓库统一的本机构建入口。

### `build_linux_release.sh`

用途：
- 构建 Linux 桌面版并打包为 `.deb` 和/或 `.rpm`。
- 会同步更新 `pubspec.yaml` 的版本号。
- 在 Linux 上会额外构建 `go_lib/core/sandbox/linux_hook` 下的 `libsandbox_preload.so`。

默认行为：
- 默认同时构建 DEB 和 RPM。
- 默认版本号为 `1.0.0`。
- 默认构建号为当前时间戳，格式如 `202603230900`。
- 默认包类型为 `community`。

常用命令：

```bash
./scripts/build_linux_release.sh
./scripts/build_linux_release.sh --deb -v 1.3.0 -ar amd64
./scripts/build_linux_release.sh --rpm -v 1.3.0 -t business -br acme -ar arm64
```

关键参数：
- `-v`, `--version <X.Y.Z>`: 语义化版本号
- `-bn`, `--build`, `--build-number <STAMP>`: 构建号
- `-ar`, `--arch <x86_64|amd64|arm64>`: 目标架构
- `-t`, `--type <community|business>`: 包类型
- `-br`, `--brand <NAME>`: business 包的品牌后缀
- `--deb`: 只构建 DEB
- `--rpm`: 只构建 RPM
- `--all`: 同时构建 DEB 和 RPM

输出：
- 中间目录：`build/linux_packaging/`
- 最终产物：`build/ClawdSecbot-<version>-<build>-<arch>-<type>[-brand].deb`
- 最终产物：`build/ClawdSecbot-<version>-<build>-<arch>-<type>[-brand].rpm`

限制：
- 不支持 Linux 跨架构构建，目标架构必须与当前主机架构一致。

### `build_macos_release_new.sh`

用途：
- macOS 统一发布脚本，覆盖 build、sign、package、notarize、upload 全流程。
- 支持 `community`、`business`、`appstore` 三种渠道。

典型阶段：
- `all`: 完整流程
- `test`: 本地调试构建，跳过上传和公证
- `build`: 仅构建
- `sign`: 仅签名
- `package`: 仅打包
- `notarize`: 仅公证
- `upload`: 仅上传 App Store

常用命令：

```bash
./scripts/build_macos_release_new.sh --type community --stage all -v 1.2.3 -bn 202603230900
./scripts/build_macos_release_new.sh --type appstore --stage all -v 1.2.3 -bn 202603230900
./scripts/build_macos_release_new.sh --type community --stage test -v 1.2.3 -bn 202603230900 -ar arm64
```

关键参数：
- `-t`, `--type <community|business|appstore>`: 发布渠道
- `--stage <all|test|build|sign|package|notarize|upload>`: 执行阶段
- `-v`, `--version <X.Y.Z>`: 版本号
- `-bn`, `--build`, `--build-number <STAMP>`: 构建号
- `-ar`, `--arch <universal|x86_64|arm64>`: 目标架构
- `-br`, `--brand <NAME>`: business 品牌后缀
- `--work-dir <dir>`: 输出根目录，默认 `build`
- `--pkg-path <path>`: 单独上传或公证时指定包路径

输出：
- 输出目录：`build/build_macos_<type>/`
- 包名格式：`ClawdSecbot-<version>-<build>-<arch>-<type>[-brand]`
- `community` / `business` 通常产出 `.dmg`
- `appstore` 通常产出 `.pkg`

依赖说明：
- 需要 macOS 原生签名、公证和上传工具链，例如 `xcrun`、`security`、有效证书和可用的 Apple 凭据。
- 证书和凭据可以通过命令参数或环境变量覆盖。

### `build_windows_release.ps1`

用途：
- 构建 Windows 版 Go 动态库和 Flutter 桌面程序。
- 将产物打包为自定义安装器 `.exe` 或分发 `.zip`。
- 默认打包格式为安装器 EXE。

常用命令：

```powershell
.\scripts\build_windows_release.ps1
.\scripts\build_windows_release.ps1 --zip
.\scripts\build_windows_release.ps1 --version 1.3.0 --build 202603230900
.\scripts\build_windows_release.ps1 --version 1.3.0 --type business --brand acme
```

关键参数：
- `-v`, `--version <X.Y.Z>`: 版本号
- `-bn`, `--build`, `--build-number <STAMP>`: 构建号
- `-ar`, `--arch <x86_64>`: 目标架构，当前仅支持 `x86_64`
- `-t`, `--type <community|business>`: 包类型
- `-br`, `--brand <NAME>`: business 品牌后缀
- `--exe`: 构建安装器 EXE，默认值
- `--zip`: 构建 ZIP 包
- `--force-pub-get`: 强制执行 `flutter pub get`

输出：
- 中间目录：`build/windows_release/`
- 最终产物：`build/ClawdSecbot-<version>-<build>-x86_64-<type>[-brand].exe`
- 最终产物：`build/ClawdSecbot-<version>-<build>-x86_64-<type>[-brand].zip`

依赖说明：
- 需要 Windows PowerShell 5.1+。
- 需要可用的 Go、Flutter、C# 编译器、CMake，以及 Windows 构建相关依赖。
- 安装器引导程序源码位于 `scripts/windows_installer/CustomInstallerBootstrap.cs`。

### `generate_icons.sh`

用途：
- 基于 `scripts/icon_1024.png` 生成 macOS、Windows 和托盘所需图标资源。

用法：

```bash
./scripts/generate_icons.sh
```

生成内容：
- `macos/Runner/Assets.xcassets/AppIcon.appiconset/` 下的 PNG 和 `AppIcon.icns`
- `images/tray_icon.png`
- `images/tray_icon.ico`（如果已安装 ImageMagick）
- `windows/runner/resources/app_icon.ico`（如果已安装 ImageMagick）

依赖说明：
- 依赖 macOS 自带的 `sips` 和 `iconutil`
- 如需生成 `.ico`，还需要 `magick` 或 `convert`

### `run_with_pprof.sh`

用途：
- 在 macOS 或 Linux 上以 pprof 模式启动桌面应用。
- 启动前会先构建 Go 插件。
- 在 Linux 上还会重新编译并安装 `libsandbox_preload.so`。

用法：

```bash
./scripts/run_with_pprof.sh
./scripts/run_with_pprof.sh 9090
./scripts/run_with_pprof.sh 6060 business
BOTSEC_PPROF_PORT=8080 ./scripts/run_with_pprof.sh
```

参数说明：
- 第一个位置参数：pprof 端口，默认 `6060`
- 第二个位置参数：构建类型，支持 `community` 或 `business`

附加行为：
- 自动检测 Flutter 目标平台：`macos` 或 `linux`
- Linux 下会安装开发用 `.desktop` 文件和多尺寸图标

### `run_with_pprof.ps1`

用途：
- 在 Windows 上以 pprof 模式启动桌面应用。
- 默认会先构建 Go 插件，可通过参数跳过。

用法：

```powershell
.\scripts\run_with_pprof.ps1
.\scripts\run_with_pprof.ps1 -PprofPort 9090
.\scripts\run_with_pprof.ps1 -Type business
.\scripts\run_with_pprof.ps1 -SkipBuild
```

参数说明：
- `-PprofPort`: pprof 端口，默认 `6060`
- `-Type <community|business>`: 构建类型
- `-SkipBuild`: 跳过插件构建

特点：
- 启动前会尽量修正 PATH，优先注入 Go、Flutter、Git、MinGW 等工具链路径
- 会在需要时自动执行 `flutter pub get`

## Assets And Helpers / 资源与辅助文件

### `icon_1024.png`

图标源文件，供 `generate_icons.sh` 统一生成各平台图标。

### `windows_installer/CustomInstallerBootstrap.cs`

Windows 自定义安装器引导程序源码，由 `build_windows_release.ps1` 编译并与安装载荷拼接，生成最终安装器 EXE。

## Notes / 备注

- `personal` 目前在多个构建脚本中仍作为历史别名存在，但会被归一化为 `community`。
- 多个平台脚本都会自动修改或依赖 `pubspec.yaml`、`plugins/`、`build/` 等目录，运行前请确认工作区状态。
- 如果你只是在本机调试 Flutter 桌面应用，通常不需要跑完整发布脚本，先执行插件构建脚本即可。
