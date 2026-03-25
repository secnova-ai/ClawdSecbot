# ClawSecbot 全局开发规范（精简版）

> 适用范围：本文件仅保留长期稳定、跨模块共享的约束。实现细节请写入各模块文档。

## 1. 架构底线

- 架构分层：Flutter Desktop（UI/状态）+ Go（业务）+ FFI（通信）。
- Go 以 `c-shared` 构建单一动态库：`botsec.dylib` / `botsec.so` / `botsec.dll`。
- 支持平台：macOS（arm64/x86_64）、Linux（arm64/x86_64）、Windows（x86_64）。

## 2. 目录职责

- `lib/`：UI、状态管理、FFI 调用封装。
- `go_lib/main.go`：唯一 FFI 导出入口（所有 `//export` 函数放在 `package main`）。
- `go_lib/core/`：公共核心能力（`repository/service/scanner/sandbox/proxy/shepherd/skillscan/modelfactory/callback_bridge/logging`）。
- `go_lib/plugins/`：插件实现（当前包含 `openclaw`、`nullclaw`）。
- `go_lib/chatmodel-routing/`：LLM 协议适配与转发。

## 3. 代码规则

- 日志和注释统一英文。
- 禁止无用代码、重复实现、无需求依据的“提前设计”。
- 优先在原有模块扩展，避免平行新实现造成语义分裂。
- Go 业务改动需补充单元测试（至少覆盖 `service/repository` 相关逻辑）。
- Flutter 生产日志统一使用 `appLogger`；Go 使用 `core/logging`。
- 非明确需求不做隐式向前兼容与数据迁移。
- 单文件代码不超过1500行

## 4. 分层与调用链

- 统一调用链：`Flutter -> FFI(main.go) -> core/service -> core/repository -> SQLite`。
- Flutter 禁止直接操作业务数据库文件，业务数据读写必须走 Go FFI。
- 资产实例隔离统一以 `asset_id` 为主键；`asset_name` 仅用于资产类型标识。

## 5. FFI 通信规范

- 统一 JSON 输入输出，响应格式固定为：
  `{"success": bool, "data": any, "error": string|null}`。
- Go 端必须使用 `json.Marshal` 序列化；禁止手工拼接 JSON。
- Go 返回的 C 字符串必须由 Dart 侧 `FreeString` 释放。
- `NativeLibraryService` 是 Flutter 侧动态库加载唯一入口：
  主 Isolate 复用缓存实例；后台 Isolate 通过 `libraryPath` 重新打开。
- Go -> Dart 消息优先使用 FFI callback（`MessageBridgeService` + `NativeCallable.listener`），轮询仅作为降级路径。

## 6. 插件与资产规范

- 插件必须实现 `BotPlugin` 并在 `init()` 中注册到 `PluginManager`。
- `BotPlugin` 关键方法组：
  `GetID/GetAssetName/GetManifest/GetAssetUISchema/ScanAssets/AssessRisks/MitigateRisk/StartProtection/StopProtection/GetProtectionStatus`。
- 每个资产实例必须生成稳定 `asset_id`（由名称、配置路径、端口、进程路径等指纹计算）。
- 运行时绑定关系：`1 asset_id : 1 plugin instance`。
- 防护、事件、指标、状态查询必须按 `asset_id` 路由，避免跨实例串数据。

### 6.1 `asset_id` 生成规范

- 统一调用 `core.ComputeAssetID(name, configPath, ports, processPaths)`，禁止插件私自实现另一套算法。
- 参与指纹字段：`name`、`config_path`、`ports`、`process_paths`。
- 规范化与拼接顺序（必须一致）：
  - `name` 转小写后写入：`name=<lowercase_name>`
  - `config_path` 非空时写入：`config=<config_path>`
  - `ports` 升序排序后写入：`ports=1,2,3`
  - `process_paths` 字典序排序后写入：`paths=/a,/b`
  - 使用 `|` 拼接为 canonical 字符串
- 哈希算法：`sha256(canonical)`，取前 6 字节转十六进制（12 位）作为短哈希。
- 输出格式：`<lowercase_name>:<12hex>`（示例：`openclaw:1a2b3c4d5e6f`）。
- 语义要求：同一资产在字段不变时 `asset_id` 必须稳定；任一指纹字段变化必须触发 `asset_id` 变化。

## 7. 监控与审计

- 审计日志必须包含请求、响应、模型、风险判断、动作、token 用量、耗时等核心字段。
- 回调模式下日志/指标/安全事件实时推送；审计日志按周期从 Go 缓冲同步入库。
- 安全事件查询、统计、清理按 `asset_id` 过滤。

## 8. 沙箱规范

- 策略目录统一：
  - Unix：`~/.botsec/policies/`
  - Windows：`%USERPROFILE%\.botsec\policies\`
- macOS：`sandbox-exec`（Seatbelt 策略）。
- Linux：`LD_PRELOAD` + JSON policy（`SANDBOX_POLICY_FILE`）。
- Windows：`sandbox_hook.dll` + MinHook 注入（失败必须 fail-close，不允许“伪保护”状态）。
- 沙箱不可用时允许降级启动，但 UI/状态必须明确标识未启用沙箱防护。

## 9. 构建与 SCM

- 常用脚本：
  - `scripts/build_go.sh`
  - `scripts/build_openclaw_plugin.sh`
  - `scripts/build_macos_release_new.sh`
  - `scripts/build_windows_release.ps1`
  - `scripts/build_linux_release.sh`
  - `scripts/generate_icons.sh`
- cgo 生成的 `.h` 和构建产物（`.dylib/.so/.dll`）不纳入版本管理。
- 禁止提交日志、缓存、平台打包中间产物。

## 10. 规范维护原则

- 本文档只保留“稳定约束”，避免写易过时实现细节。
- 若规范与代码行为冲突，以当前主干代码为准，并在同次提交更新规范。
