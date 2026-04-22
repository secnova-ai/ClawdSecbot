# ClawSecbot

**[English](README.md)**

迁移文档：
- [Version Upgrade Migration Guide (EN)](mds/version_upgrade_migration_en.md)
- [版本升级迁移指南 (ZH-CN)](mds/version_upgrade_migration_zh-CN.md)

面向 Bot 类端侧智能体的桌面 + WebUI 安全防护软件。

ClawSecbot 监控并保护运行在本地的 AI Bot 类智能体，如 Openclaw。它作为 AI Agent 与 LLM 服务之间的安全防护层 —— 拦截 API 请求、实时分析风险、执行沙箱策略，并提供完整的审计追踪。

## 媒体报道

[![CCTV 视频](assets/cctv-cover.jpg)](https://tv.cctv.com/2026/03/13/VIDEQDQA6sAow70utAbcLmMN260313.shtml)

## 功能特性

- **资产发现** — 自动扫描识别系统上的 AI Bot 进程、工作区、配置文件和端口
- **风险评估** — 对检测到的资产进行安全风险评估，包括 Skill/工具安全分析
- **防护代理** — 拦截 Bot 到 LLM 的 API 流量，分析请求/响应内容中的危险操作，在执行前向用户告警
- **沙箱防护** — 将 Bot 进程限制在操作系统级沙箱内（macOS Seatbelt / Linux LD_PRELOAD Hook / Windows MinHook），沙箱被绕过时自动恢复
- **LLM 协议转换** — 以 OpenAI 兼容格式代理请求，自动转换为各 LLM 厂商的原生协议
- **审计日志** — 记录所有请求、工具调用、风险检测和 Token 用量，支持完整的链路追踪
- **插件架构** — 可扩展的插件系统，支持不同类型的 Bot
- **WebUI 模式** — 通过 Go Web Bridge（`botsec_webd`）在浏览器中运行，同源提供 API 与静态页面

## 支持平台

| 平台     | 目标形态 | 架构    | 状态   |
|----------|----------|---------|--------|
| macOS    | Desktop  | arm64   | 已支持 |
| macOS    | Desktop  | x86_64  | 已支持 |
| Linux    | Desktop  | arm64   | 已支持 |
| Linux    | Desktop  | x86_64  | 已支持 |
| Linux    | WebUI    | arm64   | 已支持 |
| Linux    | WebUI    | x86_64  | 已支持 |
| Windows  | Desktop  | x86_64  | 已支持 |

## 架构设计

```
┌──────────────────────────────────┐
│         Flutter Desktop          │
│       (UI + 状态管理)             │
├──────────────────────────────────┤
│              FFI                 │
│        (JSON 通信协议)            │
├──────────────────────────────────┤
│        Go 动态库                  │
│      (botsec.dylib/so/dll)       │
│  ┌───────────┬─────────────────┐ │
│  │   Core    │    插件          │ │
│  │ ┌───────┐ │  ┌───────────┐  │ │
│  │ │扫描器 │ │  │ Openclaw  │  │ │
│  │ │沙箱   │ │  │  插件     │  │ │
│  │ │代理   │ │  └───────────┘  │ │
│  │ │审计DB │ │                 │ │
│  │ └───────┘ │                 │ │
│  └───────────┴─────────────────┘ │
├──────────────────────────────────┤
│  chatmodel-routing               │
│  (LLM 协议转换层)                │
├──────────────────────────────────┤
│         SQLite (数据持久化)       │
└──────────────────────────────────┘
```

ClawSecbot 采用**前后端分离**架构：

- **Flutter Desktop** — 负责 UI 渲染、状态管理和用户交互
- **Go 动态库** — 包含全部业务逻辑，编译为单一动态库（`botsec.dylib` / `botsec.so` / `botsec.dll`）
- **FFI 通信** — Flutter 通过 FFI 调用 Go 函数，使用统一 JSON 协议；Go 通过原生回调向 Flutter 推送事件

WebUI 模式复用同一套 Go Core 与插件能力，通过 Go Web Bridge 对外提供服务：

- **Flutter Web** — 由 `lib/main_web.dart` 构建静态页面
- **Go Web Bridge** — `go_lib/cmd/botsec_webd` 同时提供 HTTP API 与静态资源服务
- **HTTP 通信** — 浏览器端通过同源 HTTP 接口与后端交互

## 技术栈

| 层级     | 技术                           |
|----------|-------------------------------|
| UI       | Flutter (Desktop + WebUI)     |
| 业务逻辑 | Go (CGO, c-shared)            |
| 数据库   | SQLite (via modernc.org/sqlite) |
| 进程通信 | FFI + JSON 协议                |
| 状态管理 | Provider                       |
| 国际化   | Flutter Localizations          |
| 沙箱     | macOS Seatbelt / Linux LD_PRELOAD Hook / Windows MinHook |
| LLM SDK  | Eino 框架 (CloudWeGo)          |

### 支持的 LLM 厂商

OpenAI · Anthropic (Claude) · DeepSeek · Google (Gemini) · Ollama · Moonshot · xAI (Grok)

## 项目结构

```
bot_sec_manager/
├── lib/                        # Flutter 应用
│   ├── main.dart               # 应用入口
│   ├── main_web.dart           # Web UI 入口
│   ├── services/               # FFI 服务层
│   │   ├── native_library_service.dart
│   │   ├── plugin_service.dart
│   │   ├── protection_service.dart
│   │   ├── protection_monitor_service.dart
│   │   ├── message_bridge_service.dart
│   │   ├── sandbox_service.dart
│   │   └── *_database_service.dart
│   ├── pages/                  # UI 页面
│   ├── widgets/                # 可复用 UI 组件
│   ├── web/                    # Web UI 页面与流程
│   ├── models/                 # 数据模型
│   ├── l10n/                   # 国际化资源
│   └── utils/                  # 工具类
├── go_lib/                     # Go 安全引擎
│   ├── main.go                 # 动态库入口，导出所有 FFI 函数
│   ├── core/                   # 核心公共包
│   │   ├── plugin.go           # BotPlugin 接口定义
│   │   ├── plugin_manager.go   # 插件注册与生命周期管理
│   │   ├── path_manager.go     # 路径管理器
│   │   ├── ffi.go              # FFI 辅助函数
│   │   ├── logging/            # 日志模块
│   │   ├── repository/         # 数据访问层
│   │   ├── service/            # 业务服务层
│   │   ├── scanner/            # 资产扫描引擎
│   │   ├── sandbox/            # 沙箱策略模块
│   │   ├── webbridge/          # WebUI HTTP 桥接（API/会话）
│   │   └── callback_bridge/    # FFI 回调桥接
│   ├── plugins/openclaw/       # Openclaw Bot 插件
│   ├── skillagent/             # Skill Agent 引擎
│   ├── cmd/botsec_webd/        # Go Web Bridge 入口
│   └── chatmodel-routing/      # LLM 协议转换
│       ├── adapter/            # Provider 适配器
│       ├── providers/          # 各厂商独立实现
│       │   ├── openai/
│       │   ├── anthropic/
│       │   ├── deepseek/
│       │   ├── google/
│       │   ├── ollama/
│       │   ├── moonshot/
│       │   └── xai/
│       ├── proxy.go            # 转发代理
│       ├── filter.go           # 内容过滤器
│       └── sdk/                # 协议类型定义
├── scripts/                    # 构建与部署脚本
└── macos/ linux/ windows/      # 平台运行器
```

## 环境要求

- **Flutter** >= 3.10（需启用桌面端/Web 支持）
- **Go** >= 1.25
- **Xcode**（macOS）/ **GCC**（Linux）— 用于 CGO 编译
- **CMake**（Linux 桌面端构建）

## 构建指南

### 1. 构建 Go 安全引擎

```bash
./scripts/build_go.sh
```

编译 Go 代码为平台对应的动态库：
- macOS: `go_lib/botsec.dylib`
- Linux: `go_lib/botsec.so`
- Windows: `go_lib/botsec.dll`

### 2. 开发模式启动

```bash
./scripts/run_with_pprof.sh
```

该脚本会构建 Go 引擎并启动 Flutter 应用，同时开启 pprof 性能分析，适用于本地开发和调试。

### 3. 以 WebUI 开发模式运行

```bash
./scripts/run_web_with_pprof.sh
```

可选参数：

```bash
# pprof 端口（可选位置参数）
./scripts/run_web_with_pprof.sh 6061

# API/Web 监听端口与主机
BOTSEC_WEB_API_PORT=18080 BOTSEC_WEB_API_HOST=0.0.0.0 ./scripts/run_web_with_pprof.sh
```

启动后可访问：

- 本机 Web UI：`http://127.0.0.1:18080`
- pprof 地址：`http://127.0.0.1:6060/debug/pprof/`

### 4. 运行 Flutter 桌面应用

```bash
flutter run -d macos   # 或 -d linux, -d windows
```

### 5. 构建发布包

**macOS:**
```bash
./scripts/build_macos_release.sh
```

**Linux（一次构建 Desktop + WebUI 发布产物）:**
```bash
./scripts/build_linux_release.sh
```

默认会同时构建 Desktop 与 WebUI 产物：

- Desktop 包：`build/ClawdSecbot-desktop-<version>-<build>-<arch>-<type>.deb/.rpm`
- WebUI 包：`build/ClawdSecbot-web-<version>-<build>-<arch>-<type>.deb/.rpm`
- WebUI tar 包：`build/ClawdSecbot-web-<version>-<build>-<arch>-<type>.tar.gz`

可通过 `--deb` 或 `--rpm` 仅构建一种包格式。

**Windows（自解压 EXE，需本机安装 7-Zip 与 MinGW-w64、CMake 等，见脚本前置检查）:**
```powershell
.\scripts\build_windows_release.ps1 --version 1.0.0 --build 202601011200
```
产物为 `build\ClawdSecbot-<version>-<build>-x86_64-<type>.exe`，双击可自选解压目录（默认 `%LOCALAPPDATA%\ClawdSecbot`），解压后启动 `bot_sec_manager.exe`。

## 安装方式

### macOS

从 [Releases](../../releases) 页面下载 `.dmg` 安装包，打开后将 **ClawSecbot** 拖入应用程序文件夹。

### Linux

**Debian/Ubuntu (.deb):**
```bash
sudo dpkg -i clawsecbot_*.deb
```

**通用 Linux:**
解压发布包后直接运行可执行文件。

### Windows

当前 Windows 产物为品牌化安装器 EXE，不再依赖 7-Zip 自解压界面。运行后可选择安装目录（默认 `%LOCALAPPDATA%\Programs\ClawdSecbot`），并勾选桌面快捷方式与开始菜单快捷方式；如果检测到已有安装，安装器会提示是否执行升级覆盖，仅替换程序文件并保留用户数据与现有配置。

从 [Releases](../../releases) 下载 `ClawdSecbot-*.exe` 自解压包，运行后选择解压目录（默认用户本地应用数据下的 `ClawdSecbot` 文件夹），完成后将启动 `bot_sec_manager.exe`。也可在解压目录中稍后手动运行该可执行文件。

## 卸载说明

> ⚠️ **重要提示：** 卸载 ClawSecbot 前，请先在托盘菜单中点击**「恢复初始配置」**并重启 Openclaw。
>
> ClawSecbot 运行时会修改 `openclaw.json` 配置文件。卸载前还原配置可确保你的 Openclaw 在没有 ClawSecbot 的情况下仍能正常运行。

## 模块说明

### 核心模块 (`go_lib/core/`)

所有插件共享的基础设施：

| 模块 | 说明 |
|------|------|
| `plugin.go` | `BotPlugin` 接口 — 定义所有 Bot 插件的标准协议，包含资产发现、风险评估、防护控制和风险缓解 |
| `plugin_manager.go` | 插件注册中心，支持自动注册、重复检测和聚合 FFI 方法 |
| `scanner/` | 资产发现引擎 — 扫描 Bot 进程、端口和配置 |
| `sandbox/` | 操作系统沙箱管理 — 生成并应用 Seatbelt/LD_PRELOAD/Windows Hook 策略 |
| `repository/` | 数据访问层 — SQLite CRUD 操作 |
| `service/` | 业务逻辑 — 防护、审计、指标、版本检查 |
| `webbridge/` | Web Bridge 服务 — 为 WebUI 提供 HTTP API、会话锁和静态资源服务 |
| `callback_bridge/` | FFI 回调机制 — Go 向 Dart 推送事件 |
| `logging/` | 结构化日志 |
| `path_manager.go` | 集中路径管理 |

### 协议转换层 (`go_lib/chatmodel-routing/`)

LLM 协议转换模块：

- 从防护代理接收 **OpenAI 兼容格式** 的请求
- 转换并转发至目标 LLM 厂商的原生 API
- 将响应转换回 OpenAI 格式返回
- 支持流式输出、推理过程、工具调用和用量统计

### Skill Agent (`go_lib/skillagent/`)

负责解析、加载和安全执行 Bot 的 Skill/工具，包含 Skill 定义的安全分析功能。

### 插件 (`go_lib/plugins/`)

每个插件实现 `BotPlugin` 接口：

```go
type BotPlugin interface {
    // 基础信息
    GetAssetName() string
    
    // 资产发现
    ScanAssets() ([]Asset, error)
    
    // 风险评估
    AssessRisks(scannedHashes map[string]bool) ([]Risk, error)
    MitigateRisk(riskInfo string) string
    
    // 防护控制（支持多实例）
    StartProtection(assetID string, config ProtectionConfig) error
    StopProtection(assetID string) error
    GetProtectionStatus(assetID string) ProtectionStatus
}

// 可选生命周期钩子
type ProtectionLifecycleHooks interface {
    OnProtectionStart(ctx *ProtectionContext) (map[string]interface{}, error)
    OnBeforeProxyStop(ctx *ProtectionContext)
}
```

插件通过 `init()` 自动注册，由 `PluginManager` 统一管理。插件系统支持：

- **自动注册与重复检测** — 插件在 `init()` 中自注册，已注册则自动跳过
- **多实例资产支持** — 防护方法接受 `assetID` 参数，支持每个资产实例独立状态管理
- **生命周期钩子** — `ProtectionLifecycleHooks` 接口支持启动前/停止后的自定义操作
- **风险缓解路由** — 风险自动标记 `SourcePlugin`，确保路由回原始插件处理

Openclaw 类 Bot 适配说明文档：

- [Openclaw 类 Bot 插件适配指南](mds/openclaw_like_bot_plugin_guide_zh-CN.md)

## 参与贡献

欢迎贡献代码，请确保：

1. 遵循现有代码风格
2. 为 Go 业务逻辑编写单元测试
3. 单文件不超过 1500 行
4. Go 中所有 JSON 序列化使用 `json.Marshal`（禁止 `fmt.Sprintf` 拼接）
5. 提交前运行 `flutter analyze` 和 `go vet`

## 许可证

[GNU GPLv3](LICENSE)

### Windows 权限要求

- Windows 端 `bot_sec_manager.exe` 使用 `requireAdministrator` 清单请求权限
- 所有构建类型（Debug/Profile/Release）启动时都会触发 UAC 提示
- 若拒绝 UAC 无法获取管理员权限，应用将立即退出（fail-close 机制）
