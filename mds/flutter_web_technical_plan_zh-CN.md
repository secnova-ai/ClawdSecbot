# Flutter Web 技术方案（Linux 优先）

## 1. 目标与约束

### 1.1 目标
- 在不改变 Go 业务逻辑的前提下，为项目新增 Flutter Web 版本。
- 保持 Desktop 版本行为不变。
- Web 版本优先支持 Linux 运行与打包。
- 提供与 `./scripts/run_with_pprof.sh` 对齐的 Web 调试脚本。
- Web 版本也支持“路径前缀传入 Go 侧做路径富化”。
- 继续支持国际化（`zh/en`）。

### 1.2 硬约束（来自仓库规则）
- 架构边界必须保持：`Flutter -> (FFI/Bridge) -> core/service -> repository -> SQLite`。
- `go_lib/main.go` 仍是唯一 FFI 导出入口（不改此原则）。
- JSON 输入输出协议保持统一；Go 侧序列化使用 `json.Marshal`。
- 不引入“为了历史兼容而扩散的大量兼容代码”。
- 不修改业务逻辑，仅做通信与 UI 形态适配。

---

## 2. 现状基线

- Go FFI 导出入口：`go_lib/main.go`
- Flutter FFI 入口服务：`lib/services/native_library_service.dart`
- 当前 UI 直接依赖 `dart:ffi` / `dart:io` / `window_manager` / `tray_manager`，Web 无法直接编译。
- 当前调试脚本：`scripts/run_with_pprof.sh`
- 当前路径富化入口：`InitPathsFFI(workspaceDir, homeDir)` -> `core.Initialize(...)`

统计（基于当前代码扫描）：
- Go `//export` 总数：`130`
- Flutter 代码中已使用导出函数：`118`
- Flutter 当前未使用导出函数：`12`

---

## 3. 目标架构

### 3.1 Desktop（保持不变）
`Flutter Desktop -> FFI(main.go exports) -> core/service -> repository -> SQLite`

### 3.2 Web（新增）
`Flutter Web -> HTTP/SSE Bridge(botsec_webd) -> core/service -> repository -> SQLite`

说明：
- `go_lib/main.go` 不新增 `//export`，继续作为 Desktop FFI 入口。
- 新增 Linux 进程 `botsec_webd`（Go 命令）作为 Web 通信桥接层。
- Bridge 仅做“协议适配与转发”，不承载业务判断。

---

## 4. 通信抽象设计

### 4.1 Dart 侧统一接口
新增抽象接口（示意）：

```dart
abstract class BotsecTransport {
  Future<Map<String, dynamic>> callNoArg(String method);
  Future<Map<String, dynamic>> callOneArg(String method, String arg);
  Future<Map<String, dynamic>> callTwoArgs(String method, String arg1, String arg2);

  Stream<Map<String, dynamic>> get eventStream; // log/metrics/security_event/truth_record/version_update
  Future<void> initialize(BotsecBootstrapContext context);
  Future<void> dispose();
}
```

实现：
- `FfiTransport`：复用现有 FFI 行为（Desktop）。
- `HttpTransport`：调用 `botsec_webd` 的 RPC + SSE（Web）。

### 4.2 方法映射策略
- 保持“方法名等价”：
  - FFI: `ScanAssetsFFI`
  - HTTP: `POST /api/v1/rpc/ScanAssetsFFI`
- 参数编码规则：
  - `no-arg`: `{}`
  - `one-arg`: `{"arg": "..."}`
  - `two-arg`: `{"arg1":"...","arg2":"..."}`
- 返回值保持现有 JSON envelope：
  - `{"success": bool, "data": any, "error": string|null}`

### 4.3 实时消息映射
- Desktop：`RegisterMessageCallback`（FFI callback）
- Web：`GET /api/v1/events`（SSE）
- 消息结构保持与 `callback_bridge.Message` 一致：
  - `type`: `log|metrics|status|version_update|security_event|truth_record`
  - `timestamp`
  - `payload`

---

## 5. 路径前缀与初始化方案（满足 Web 需求）

Web 启动顺序：
1. Web UI 获取初始化上下文（默认值可由服务端提供）。
2. Web UI 显式调用初始化接口并传入路径前缀：
   - `workspace_dir_prefix`
   - `home_dir`
3. Go Bridge 内部调用：
   - `core.Initialize(workspaceDir, homeDir)`
   - `core.InitLogging(logDir)`
   - `service.InitializeDatabase(current_version)`

建议新增接口：
- `POST /api/v1/bootstrap/init`
- body:
  - `workspace_dir_prefix`
  - `home_dir`
  - `current_version`

这样可与 Desktop `InitPathsFFI + InitLoggingFFI + InitDatabase` 语义对齐。

---

## 6. API 覆盖策略

### 6.1 V1 必须覆盖
- 覆盖 Flutter 当前已使用的 `118` 个导出函数（按当前扫描结果）。
- 保证 Web/Desktop 相同输入下，核心业务 JSON 输出语义一致。

### 6.2 当前未使用但保留的导出函数（12）
- `ClearAllAuditLogsByFilterFFI`
- `GetAllProtectionStatusFFI`
- `GetAllSandboxStatus`
- `GetAuditLogStatisticsByFilterFFI`
- `GetProtectionStatusFFI`
- `GetShepherdRulesFFI`
- `StartProtectionFFI`
- `StopProtectionFFI`
- `SyncGatewaySandboxByAssetName`
- `UpdateBotForwardingProvider`
- `UpdateShepherdRulesFFI`
- `WaitForProtectionLogs`

策略：
- Bridge 端同样提供这些 RPC 路由（即便 UI 当前未调用），避免未来功能扩展再改协议层。

---

## 7. UI 形态调整（Web 适配无图形 Linux）

- 新增 Web 入口：`lib/main_web.dart`。
- 不复用托盘和多窗口交互；改为单页多视图（Tab/Route）：
  - 资产扫描
  - 防护监控
  - 审计日志
  - 设置/语言
- Desktop 入口 `lib/main.dart` 保持不变。

关键点：
- 不改业务流程，仅改展示与交互容器。
- 不引入跨平台“兜底 if-else 大兼容层”；Web 独立入口和页面骨架。

---

## 8. 国际化方案

- 继续使用现有 ARB 与 `AppLocalizations`（`lib/l10n/arb`）。
- Web 首次加载从后端读取语言（等价 `GetLanguageFFI`）。
- 语言切换写回后端（等价 `SetLanguageFFI`），并触发版本检查语言同步（等价 `UpdateVersionCheckLanguageFFI`）。

---

## 9. Linux 调试与打包

### 9.1 调试脚本（新增）
新增：`scripts/run_web_with_pprof.sh`

行为：
1. 构建 Go（含现有插件/沙箱 preload 构建流程）。
2. 启动 `botsec_webd`（支持 `BOTSEC_PPROF_PORT`，默认 6060）。
3. 启动 Flutter Web 调试（建议 `flutter run -d web-server -t lib/main_web.dart`）。
4. 输出 pprof 地址与访问 URL。

### 9.2 Linux 打包（新增）
新增：`scripts/build_linux_web_release.sh`

产物建议（V1）：
- `botsec_webd` 二进制
- `flutter build web -t lib/main_web.dart` 产物
- 启动脚本（前后端一键拉起）
- `tar.gz`（V1），DEB/RPM 后续补充

---

## 10. 分阶段实施计划

### Phase 1：通信抽象落地（不改业务）
- 新增 `BotsecTransport` + `FfiTransport`。
- 将现有服务中的 FFI 直连调用收敛到 transport。
- Desktop 行为保持一致（零回归）。

### Phase 2：Go Web Bridge
- 新增 `go_lib/cmd/botsec_webd`。
- 实现 RPC 路由和 SSE 推送。
- 实现 bootstrap 初始化（含路径前缀传入）。

### Phase 3：Web UI
- 新增 `lib/main_web.dart` 和 Web 页面骨架。
- 接入 `HttpTransport`。
- 完成关键页面（扫描/防护/审计/设置）。

### Phase 4：脚本与打包
- 增加 `run_web_with_pprof.sh`。
- 增加 Linux Web 打包脚本。

### Phase 5：回归与验收
- Desktop 全回归。
- Web 功能验收。
- 协议一致性回归。

---

## 11. 最小改动文件清单（实施基线）

### 11.1 必新增
- `go_lib/cmd/botsec_webd/main.go`
- `go_lib/core/webbridge/`（router/handlers/events/bootstrap）
- `lib/core_transport/botsec_transport.dart`
- `lib/core_transport/ffi_transport.dart`
- `lib/core_transport/http_transport.dart`
- `lib/core_transport/bridge_event_stream.dart`
- `lib/main_web.dart`
- `scripts/run_web_with_pprof.sh`
- `scripts/build_linux_web_release.sh`
- `mds/flutter_web_technical_plan_zh-CN.md`（本文档）

### 11.2 必修改（第一批）
- `lib/services/native_library_service.dart`（抽象接入点）
- `lib/services/message_bridge_service.dart`（抽象事件源）
- `lib/services/plugin_service.dart`
- `lib/services/protection_service.dart`
- `lib/services/protection_monitor_service.dart`
- `lib/services/protection_database_service.dart`
- `lib/services/audit_log_database_service.dart`
- `lib/services/security_event_database_service.dart`
- `lib/services/metrics_database_service.dart`
- `lib/services/app_settings_database_service.dart`
- `lib/services/scan_database_service.dart`
- `lib/services/provider_service.dart`
- `lib/services/sandbox_service.dart`
- `lib/services/skill_security_analyzer_service.dart`

### 11.3 必修改（Web 入口相关）
- `pubspec.yaml`（Web 启动入口相关脚本/资源校验，不改业务依赖逻辑）

---

## 12. 风险与控制

- 风险：Desktop 回归
  - 控制：Phase 1 仅做 transport 收敛，先保持 `FfiTransport` 全量兼容。
- 风险：Web 实时事件延迟
  - 控制：SSE + 心跳 + 自动重连；服务端缓存窗口可配置。
- 风险：路径初始化不一致
  - 控制：统一走 bootstrap，断言初始化成功后才开放业务 API。
- 风险：无图形 Linux 部署复杂
  - 控制：提供一键启动脚本与单包发布物。

---

## 13. 验收标准

- Desktop：
  - 全功能可用，行为与改造前一致。
- Web（Linux）：
  - 可完成扫描、风险评估、防护启停、监控、审计、语言切换。
  - 路径前缀可由 Web 初始化传入并生效。
  - 支持 pprof 调试模式。
- 一致性：
  - 核心接口在 Desktop/Web 下返回 JSON 语义一致。


---

## 14. 附录：API 覆盖清单文件

- 已使用导出函数清单（Web V1 必须覆盖）：
  - `mds/flutter_web_api_used_functions.txt`（118 项）
- 当前未使用导出函数清单（建议也提供路由）：
  - `mds/flutter_web_api_unused_functions.txt`（12 项）

说明：
- 两个清单由当前代码自动统计生成，可用于后续 CI 校验 Web Bridge 覆盖率。

