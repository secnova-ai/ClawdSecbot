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
- 路径统一由 core 派生；Flutter 仅传基础目录，禁止分别传 db/log/version 等具体路径。

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

## 7. 监控与审计 — TruthRecord SSOT 架构

### 7.1 核心原则

代理防护层（ProxyProtection）的核心数据实体为 **TruthRecord**（Single Source of Truth）。
每个代理请求有且仅有一条 TruthRecord，从请求到达到响应完成渐进式更新。
TruthRecord 同时服务于三个视图：

1. **清晰视图卡片**：实时展示请求状态、摘要、工具调用、安全决策
2. **审计日志**：已完成请求的持久化记录（SQLite）
3. **安全事件过滤**：按风险等级筛选异常请求

禁止为上述三个视图维护独立的数据结构或并行写入路径。

### 7.2 数据模型

**Go 侧** (`go_lib/core/proxy/truth_record.go`)：

| 字段组 | 字段 | 说明 |
|--------|------|------|
| 身份 | `request_id`, `asset_name`, `asset_id` | 请求唯一标识与资产归属 |
| 时间线 | `started_at`, `updated_at`, `completed_at` | RFC3339Nano 格式；`completed_at` 非空即表示已完成 |
| 请求上下文 | `model`, `message_count`, `messages[]` | `messages` 仅含当前轮 |
| 响应 | `phase`, `finish_reason`, `primary_content`, `primary_content_type`, `output_content` | `primary_content` 为卡片摘要；`output_content` 为审计全文 |
| 工具链路 | `tool_calls[]` | 每项含 `name/arguments/result/source/is_sensitive` |
| 安全决策 | `decision` | 子结构 `SecurityDecision{action, risk_level, reason, confidence}` |
| Token | `prompt_tokens`, `completion_tokens` | `total_tokens` 由前端 getter 计算 |

**Flutter 侧** (`lib/models/truth_record_model.dart`)：

- `TruthRecordModel` 直接从 Go 快照映射，每次收到完整快照直接替换（不做 merge）。
- 所有可推导属性通过 getter 计算：`isComplete`, `hasToolCall`, `toolCallCount`, `toolNames`, `totalTokens`, `durationMs`, `hasRisk`, `decisionBlocked` 等。

### 7.3 数据流

```
Bot 请求 → ProxyProtection.onRequest()
  → updateTruthRecord() 创建记录 (phase=starting)
  → RecordStore.Upsert() → 快照入 pending 队列
  → logTruthRecordSnapshot() → 写入视图日志
  → sendTruthRecordToCallback() → FFI 推送 Flutter
  → sendLog("protection_record_snapshot") → 原始日志流

Provider 响应 → onResponse() / onStreamChunk()
  → updateTruthRecord() 更新记录
  → finalizeTruthRecord() 标记完成 (phase=completed)

Flutter 侧
  → MessageBridgeService.truthRecordStream 接收快照
  → ProtectionMonitorService 分发到 truthRecordStream
  → 卡片渲染：直接替换 requestGroups[requestId]
  → 持久化：isComplete 时转换为 AuditLog 写入 SQLite
```

### 7.4 开发约束

- **唯一写入入口**：handler 中更新请求数据必须通过 `updateTruthRecord()`，禁止绕过直接操作 `RecordStore`。
- **禁止并行数据结构**：不允许新增 `RequestView`、`AuditLog`、`RequestSnapshot` 等平行实体。
- **快照不可变**：`RecordStore.Upsert()` 返回的快照为深拷贝，外部持有者不可修改。
- **安全决策内聚**：风险相关字段必须收敛在 `SecurityDecision` 子结构中，禁止在 TruthRecord 顶层散列安全字段。
- **前端不做 merge**：每次收到快照直接替换，不与本地旧数据合并。
- **FFI 兼容层**：`truthRecordsToAuditCompat()` 为过渡期兼容函数，最终应由 Flutter 直接使用 `TruthRecordModel`。
- 安全事件（`SecurityEvent`）来自 ShepherdGate 分析和沙箱钩子，与 `TruthRecord.Decision` 是独立数据源，不合并。

## 8. 沙箱规范

- 策略目录统一：
  - Unix：`~/.botsec/policies/`
  - Windows：`%USERPROFILE%\.botsec\policies\`
- macOS：`sandbox-exec`（Seatbelt 策略）。
- Linux：`LD_PRELOAD` + JSON policy（`SANDBOX_POLICY_FILE`）。
- Windows：`sandbox_hook.dll` + MinHook 注入（失败必须 fail-close，不允许“伪保护”状态）。
- 沙箱不可用时允许降级启动，但 UI/状态必须明确标识未启用沙箱防护。
- 实现细节与代码路径见 [`_rules/config_system.md`](config_system.md)（含各平台沙箱策略与相关 Go 模块）。
