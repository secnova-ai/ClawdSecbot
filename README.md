# ClawSecbot

**[中文文档](README_zh-CN.md)**

Migration docs:
- [Version Upgrade Migration Guide (EN)](mds/version_upgrade_migration_en.md)
- [版本升级迁移指南 (ZH-CN)](mds/version_upgrade_migration_zh-CN.md)

Desktop security protection software for Bot-type endpoint AI agents.

ClawSecbot monitors and secures local AI Bot agents (such as Openclaw) running on your machine. It acts as a protective layer between AI agents and LLM services — intercepting API requests, analyzing risks in real time, enforcing sandbox policies, and providing full audit trails.

## Community Reviews

[![Review 1](https://img.youtube.com/vi/Hr2uh5qSQmo/hqdefault.jpg)](https://m.youtube.com/watch?v=Hr2uh5qSQmo)
[![Review 2](https://img.youtube.com/vi/ACVLW2OoCO0/hqdefault.jpg)](https://www.youtube.com/watch?v=ACVLW2OoCO0&t=4s)

## Features

- **Asset Discovery** — Automatically scans and identifies AI Bot processes, workspaces, configurations, and ports on your system
- **Risk Assessment** — Evaluates detected assets for security risks, including Skill/tool security analysis
- **Protection Proxy** — Intercepts Bot-to-LLM API traffic, analyzes request/response content for dangerous operations, and alerts users before execution
- **Sandbox Enforcement** — Confines Bot processes within OS-level sandboxes (macOS Seatbelt / Linux seccomp) with auto-recovery if sandbox is bypassed
- **LLM Protocol Translation** — Proxies requests in OpenAI-compatible format and translates to/from various LLM providers
- **Audit Logging** — Records all requests, tool calls, risk detections, and token usage with full traceability
- **Plugin Architecture** — Extensible plugin system for supporting different Bot types

## Supported Platforms

| Platform | Architecture | Status |
|----------|-------------|--------|
| macOS    | arm64       | Supported |
| macOS    | x86_64      | Supported |
| Linux    | arm64       | Supported |
| Linux    | x86_64      | Supported |
| Windows  | x86_64      | Supported |

### Windows Privilege Requirement

- On Windows, `bot_sec_manager.exe` is configured with `requireAdministrator`.
- Every launch (Debug/Profile/Release) will trigger a UAC prompt.
- If UAC is denied or elevation is unavailable, the app exits immediately (fail-close).

## Architecture

```
┌──────────────────────────────────┐
│         Flutter Desktop          │
│     (UI + State Management)      │
├──────────────────────────────────┤
│              FFI                 │
│     (JSON-based Protocol)        │
├──────────────────────────────────┤
│       Go Shared Library          │
│      (botsec.dylib/so/dll)       │
│  ┌───────────┬─────────────────┐ │
│  │   Core    │    Plugins      │ │
│  │ ┌───────┐ │  ┌───────────┐  │ │
│  │ │Scanner│ │  │ Openclaw  │  │ │
│  │ │Sandbox│ │  │  Plugin   │  │ │
│  │ │Proxy  │ │  └───────────┘  │ │
│  │ │AuditDB│ │                 │ │
│  │ └───────┘ │                 │ │
│  └───────────┴─────────────────┘ │
├──────────────────────────────────┤
│  chatmodel-routing               │
│  (LLM Protocol Translation)      │
├──────────────────────────────────┤
│         SQLite (Data)            │
└──────────────────────────────────┘
```

ClawSecbot uses a **frontend-backend separation** architecture:

- **Flutter Desktop** — Handles UI rendering, state management, and user interaction
- **Go Shared Library** — Contains all business logic, compiled as a single dynamic library (`botsec.dylib` / `botsec.so` / `botsec.dll`)
- **FFI Communication** — Flutter calls Go functions via FFI with a unified JSON protocol; Go pushes events back via native callbacks

## Tech Stack

| Layer     | Technology                    |
|-----------|-------------------------------|
| UI        | Flutter Desktop (Dart)        |
| Logic     | Go (CGO, c-shared)           |
| Database  | SQLite (via modernc.org/sqlite) |
| IPC       | FFI + JSON protocol           |
| State     | Provider                      |
| i18n      | Flutter Localizations         |
| Sandbox   | macOS Seatbelt / Linux seccomp |
| LLM SDK   | Eino framework (CloudWeGo)    |

### Supported LLM Providers

OpenAI · Anthropic (Claude) · DeepSeek · Google (Gemini) · Ollama · Moonshot · xAI (Grok)

## Project Structure

```
bot_sec_manager/
├── lib/                        # Flutter application
│   ├── main.dart               # App entry point
│   ├── services/               # FFI service layer
│   │   ├── native_library_service.dart
│   │   ├── plugin_service.dart
│   │   ├── protection_service.dart
│   │   ├── protection_monitor_service.dart
│   │   ├── message_bridge_service.dart
│   │   ├── sandbox_service.dart
│   │   └── *_database_service.dart
│   ├── pages/                  # UI pages
│   ├── widgets/                # Reusable UI components
│   ├── models/                 # Data models
│   ├── l10n/                   # Internationalization
│   └── utils/                  # Utilities
├── go_lib/                     # Go security engine
│   ├── main.go                 # Dylib entry, all FFI exports
│   ├── core/                   # Core package
│   │   ├── plugin.go           # BotPlugin interface
│   │   ├── plugin_manager.go   # Plugin registry
│   │   ├── path_manager.go     # Path management
│   │   ├── ffi.go              # FFI helpers
│   │   ├── logging/            # Logging module
│   │   ├── repository/         # Data access layer
│   │   ├── service/            # Business services
│   │   ├── scanner/            # Asset scanner
│   │   ├── sandbox/            # Sandbox policies
│   │   └── callback_bridge/    # FFI callback bridge
│   ├── plugins/openclaw/       # Openclaw Bot plugin
│   ├── skillagent/             # Skill Agent engine
│   └── chatmodel-routing/      # LLM protocol translation
│       ├── adapter/            # Provider adapter
│       ├── providers/          # Per-provider implementations
│       │   ├── openai/
│       │   ├── anthropic/
│       │   ├── deepseek/
│       │   ├── google/
│       │   ├── ollama/
│       │   ├── moonshot/
│       │   └── xai/
│       ├── proxy.go            # Forwarding proxy
│       ├── filter.go           # Content filter
│       └── sdk/                # Protocol types
├── scripts/                    # Build & deployment scripts
└── macos/ linux/ windows/      # Platform runners
```

## Prerequisites

- **Flutter** >= 3.10 (with desktop support enabled)
- **Go** >= 1.25
- **Xcode** (macOS) / **GCC** (Linux) — for CGO compilation
- **CMake** (Linux desktop builds)

## Building

### 1. Build the Go Security Engine

```bash
./scripts/build_go.sh
```

This compiles the Go code into a platform-specific shared library:
- macOS: `go_lib/botsec.dylib`
- Linux: `go_lib/botsec.so`
- Windows: `go_lib/botsec.dll`

### 2. Build the Openclaw Plugin

```bash
./scripts/build_openclaw_plugin.sh
```

### 3. Run in Development Mode

```bash
./scripts/run_with_pprof.sh
```

This script builds the Go engine and launches the Flutter app with pprof profiling enabled, suitable for local development and debugging.

### 4. Run the Flutter Application

```bash
flutter run -d macos   # or -d linux, -d windows
```

### 5. Build Release Package

**macOS:**
```bash
./scripts/build_macos_release.sh
```

**Linux (deb):**
```bash
./scripts/build_linux_deb.sh
```

**Linux (generic):**
```bash
./scripts/build_linux_release.sh
```

## Installation

### macOS

Download the `.dmg` installer from the [Releases](../../releases) page, open it, and drag **ClawSecbot** to your Applications folder.

### Linux

**Debian/Ubuntu (.deb):**
```bash
sudo dpkg -i clawsecbot_*.deb
```

**Generic Linux:**
Extract the release archive and run the executable directly.

## Uninstallation

> ⚠️ **Important:** Before uninstalling ClawSecbot, please click **"Restore Initial Configuration"** in the tray menu and restart Openclaw.
>
> ClawSecbot modifies the `openclaw.json` configuration file during runtime. Restoring the initial configuration before uninstallation ensures that your Openclaw will continue to function normally without ClawSecbot.

## Module Overview

### Core (`go_lib/core/`)

The shared foundation used by all plugins:

| Module | Description |
|--------|-------------|
| `plugin.go` | `BotPlugin` interface — defines the contract for all Bot plugins, including asset discovery, risk assessment, protection control, and mitigation |
| `plugin_manager.go` | Plugin registry with auto-registration, duplicate detection, and aggregated FFI methods |
| `scanner/` | Asset discovery engine — scans for Bot processes, ports, and configurations |
| `sandbox/` | OS sandbox management — generates and applies Seatbelt/seccomp policies |
| `repository/` | Data access layer — SQLite CRUD operations |
| `service/` | Business logic — protection, audit, metrics, version checking |
| `callback_bridge/` | FFI callback mechanism — Go-to-Dart event push |
| `logging/` | Structured logging |
| `path_manager.go` | Centralized path management |

### Chatmodel Routing (`go_lib/chatmodel-routing/`)

LLM protocol translation layer:

- Receives requests in **OpenAI-compatible** format from the protection proxy
- Translates and forwards to the target LLM provider's native API
- Converts responses back to OpenAI format
- Supports streaming, reasoning, tool calls, and usage tracking

### Skill Agent (`go_lib/skillagent/`)

Engine for parsing, loading, and securely executing Bot skills/tools. Includes security analysis of skill definitions.

### Plugins (`go_lib/plugins/`)

Each plugin implements the `BotPlugin` interface:

```go
type BotPlugin interface {
    // Basic Info
    GetAssetName() string
    
    // Asset Discovery
    ScanAssets() ([]Asset, error)
    
    // Risk Assessment
    AssessRisks(scannedHashes map[string]bool) ([]Risk, error)
    MitigateRisk(riskInfo string) string
    
    // Protection Control (per-instance)
    StartProtection(assetID string, config ProtectionConfig) error
    StopProtection(assetID string) error
    GetProtectionStatus(assetID string) ProtectionStatus
}

// Optional lifecycle hooks
type ProtectionLifecycleHooks interface {
    OnProtectionStart(ctx *ProtectionContext) (map[string]interface{}, error)
    OnBeforeProxyStop(ctx *ProtectionContext)
}
```

Plugins auto-register via `init()` and are managed through the `PluginManager`. The plugin system supports:

- **Auto-registration with duplicate detection** — Plugins register themselves in `init()`, skipped if already registered
- **Multi-instance asset support** — Protection methods accept `assetID` for per-instance state management
- **Lifecycle hooks** — `ProtectionLifecycleHooks` interface for pre-start/post-stop customization
- **Risk mitigation routing** — Risks are automatically tagged with `SourcePlugin` for proper routing to the originating plugin

Plugin adaptation guide for Openclaw-like bots:

- [Openclaw-like Bot Plugin Adaptation Guide](mds/openclaw_like_bot_plugin_guide.md)

## Contributing

Contributions are welcome. Please make sure to:

1. Follow the existing code style
2. Write unit tests for Go business logic
3. Keep files under 1500 lines
4. Use `json.Marshal` for all JSON serialization in Go (no `fmt.Sprintf`)
5. Run `flutter analyze` and `go vet` before submitting

## License

[GNU GPLv3](LICENSE)
