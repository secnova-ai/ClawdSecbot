# ClawSecbot 全局开发规范（精简版）

> 适用范围：本文件只定义跨模块、长期稳定、可执行的约束。模块内部实现细节写入模块文档。

## 0. 规则优先级与执行口径

- 本文件中的 `必须` / `禁止` 为硬约束；`建议` / `优先` 为软约束。
- 若用户在当前任务中明确要求与本规范冲突，以用户要求为准，但必须在提交中同步更新本规范或模块文档，避免“代码已变、规则未变”。
- 审查输出必须给出：`文件路径 + 触发规则 + 具体原因`，避免抽象结论。
- 单个功能（如审计日志、某插件协议、某页面交互）的实现细则禁止长期堆在 `global.md`，必须拆分到独立规则文档并在此处维护索引。

## 1. 架构底线

- 架构分层：Flutter Desktop（UI/状态）+ Go（业务）+ FFI（通信）。
- Go 以 `c-shared` 构建单一动态库：`botsec.dylib` / `botsec.so` / `botsec.dll`。
- 支持平台：macOS（arm64/x86_64）、Linux（arm64/x86_64）、Windows（x86_64）、webui。
- 任何改动都需要兼容desktop版本和webui版本的工作

## 2. 目录职责

- `lib/`：UI、状态管理、FFI 调用封装。
- `go_lib/main.go`：唯一 FFI 导出入口（所有 `//export` 函数在 `package main`）。
- `go_lib/core/`：公共核心能力（`repository/service/scanner/sandbox/proxy/shepherd/skillscan/modelfactory/callback_bridge/logging`）。
- `go_lib/plugins/`：插件实现（当前包含 `openclaw`、`nullclaw`）。
- `go_lib/chatmodel-routing/`：LLM 协议适配与转发。

## 3. 代码规则

- 日志与代码注释统一英文。
- 禁止无用代码、重复实现、无需求依据的“提前设计”。
- 优先在原有模块扩展，避免平行实现造成语义分裂。
- Go 业务改动必须补充测试；至少覆盖改动涉及的核心逻辑（`service`/`repository`/`proxy`）。
- Flutter 生产日志使用 `appLogger`；Go 使用 `core/logging`。
- 非明确需求，不做隐式前向兼容和数据迁移。
- 涉及数据库迁移逻辑的改动，必须同步更新当前应用版本号（`pubspec.yaml`），禁止“迁移已改但版本未升”。
- 单文件行数硬上限：1500 行。对已超限文件，若继续改动，必须在同次提交中拆分或净减少体积；紧急修复可豁免一次，但后续必须补拆分。
- 涉及耗时操作的实现与评审，必须遵循 [`_rules/ui_non_blocking_async.md`](ui_non_blocking_async.md)。

## 4. 分层与调用链

- 默认调用链：`Flutter -> FFI(main.go) -> core/service -> core/repository -> SQLite`。
- Flutter 禁止直接操作业务数据库文件；业务数据读写必须经 Go FFI。
- 允许的例外：`core/proxy` 在请求热路径中可异步写入 `core/repository`（仅限审计/安全事件这类旁路持久化），前提是：
  - 失败不阻断代理主流程；
  - 不新增第二套对外读取语义；
  - 表结构与查询口径保持一致；
  - 有明确日志与测试覆盖并发/乱序场景。
- 资产实例隔离以 `asset_id` 为主键；`asset_name` 仅作类型标识。

## 5. FFI 通信规范

- FFI 输入输出统一 JSON。
- 默认响应包络：`{"success": bool, "data": any, "error": string|null}`。
- 对高频历史接口（如代理日志流/快照拉取）可返回裸 JSON，但必须在接口注释中明确返回结构，且同名接口不得混用两种格式。
- Go 必须使用 `json.Marshal` 序列化；禁止手工拼接 JSON。
- Go 返回的 C 字符串必须由 Dart 侧 `FreeString` 释放。
- `NativeLibraryService` 是 Flutter 动态库加载唯一入口：主 Isolate 复用缓存实例；后台 Isolate 通过 `libraryPath` 重开。
- Go -> Dart 消息优先 FFI callback（`MessageBridgeService` + `NativeCallable.listener`），轮询仅作降级。
- 路径由 core 统一派生；Flutter 仅传基础目录，禁止传 db/log/version 等细粒度路径。

## 6. 插件与资产规范

- 插件必须实现 `BotPlugin` 并在 `init()` 注册到 `PluginManager`。
- `BotPlugin` 关键方法组：
  `GetID/GetAssetName/GetManifest/GetAssetUISchema/ScanAssets/GetMainProcessPID/AssessRisks/MitigateRisk/StartProtection/StopProtection/GetProtectionStatus`。
- 每个资产实例必须生成稳定 `asset_id`，仅由名称与配置路径参与指纹计算。
- 运行时绑定关系：`1 asset_id : 1 plugin instance`。
- 防护、事件、指标、状态查询必须按 `asset_id` 路由，避免跨实例串数据。
- FFI 防护接口只接受 `asset_id`，禁止传 `asset_name`；Go 内部通过实例绑定解析插件名。

### 6.1 `asset_id` 生成规范

- 统一调用 `core.ComputeAssetID(name, configPath)`，禁止插件自实现另一套算法。
- **仅**允许以下字段参与指纹：`name`、`config_path`。
- **禁止**将运行态动态信息（`ports`、`process_paths`、`pid`、`service_name` 等）卷入指纹，否则 bot 启停会导致 `asset_id` 漂移，出现"同一资产对应多条策略"或"启用防护后策略丢失"的问题。
- 规范化顺序：
  - `name` 小写：`name=<lowercase_name>`
  - `config_path` 非空：`config=<config_path>`
  - 使用 `|` 拼接 canonical 字符串
- 哈希算法：`sha256(canonical)`，取前 6 字节十六进制（12 位）。
- 输出格式：`<lowercase_name>:<12hex>`（例：`openclaw:1a2b3c4d5e6f`）。
- 同一 `(name, config_path)` 必须在任何运行态下产生相同 `asset_id`；`config_path` 变化必须触发 `asset_id` 变化。

### 6.2 `mitigation` 生成规范

- 所有插件的 `mitigation.json` 每条 `mitigation` 必须有 `title` 与 `description`。
- `mitigation.type` 仅允许：
  - `form`：使用 `form_schema`，表示可执行修复。
  - `suggestion`：使用 `suggestions`，表示人工修复建议。
- `form` 禁止以 `suggestions` 为主承载；`suggestion` 禁止以 `form_schema` 为主承载。
- `title` 概括目标；`description` 说明背景、目的、影响或前提，可直接用于 UI 与导出。

## 7. 监控与审计（高层约束）

- 监控（实时态）与审计（持久态）允许采用不同数据结构，但必须边界清晰、职责不重叠。
- 同一 UI 视图只允许一个主数据源，禁止在页面层混读两套语义相近的数据并做隐式合并。
- 监控/审计相关实现必须满足：并发可关联、链路可追溯、写入幂等、失败不阻断主业务。
- 详细实现规范见独立文档：
  - [`_rules/audit_chain.md`](audit_chain.md)
  - [`_rules/security_event.md`](security_event.md)

## 8. 沙箱规范

- 策略目录统一：
  - Unix：`~/.botsec/policies/`
  - Windows：`%USERPROFILE%\\.botsec\\policies\\`
- macOS：`sandbox-exec`（Seatbelt 策略）。
- Linux：`LD_PRELOAD` + JSON policy（`SANDBOX_POLICY_FILE`）。
- Windows：`sandbox_hook.dll` + MinHook 注入（失败必须 fail-close，不允许“伪保护”状态）。
- 沙箱不可用时允许降级启动，但 UI/状态必须明确标识“未启用沙箱防护”。
- 实现细节与代码路径见 [`_rules/config_system.md`](config_system.md)。
