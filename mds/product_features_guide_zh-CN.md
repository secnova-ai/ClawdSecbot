# ClawSecbot 产品功能说明

## 概述

ClawSecbot 是一款面向 AI Bot（如 Openclaw、Nullclaw、DinTalClaw 等）的桌面安全管理工具。它基于 Flutter Desktop + Go + FFI 架构，提供资产自动发现、安全风险评估、实时代理防护、沙箱隔离、技能安全扫描、审计日志等核心能力，帮助用户全面管控本地运行的 AI Bot 实例。

---

## 1. 资产扫描与发现

### 功能描述

自动扫描本地系统，发现正在运行的 AI Bot 实例。扫描器通过采集系统快照（进程、端口、配置文件等），匹配预定义的检测规则来识别不同类型的 Bot 资产。每个发现的资产会基于名称、配置路径、端口和进程路径生成唯一的 `asset_id` 指纹标识。

### 触发方式

在主界面点击扫描按钮触发。扫描完成后结果会展示在主页面中，也可设置定时扫描间隔进行周期性自动扫描。

### 代码依据
- FFI 入口：`go_lib/main.go` — `ScanAssetsFFI`
- 扫描核心：`go_lib/core/scanner/scanner.go` — `AssetScanner.Scan()`
- 系统采集器：`go_lib/core/collector_darwin.go`、`collector_linux.go`、`collector_windows.go`
- 规则引擎：`go_lib/core/engine.go` — `AssetDetectionEngine`
- 资产模型：`go_lib/core/asset.go` — `Asset`、`ComputeAssetID()`
- Flutter 扫描服务：`lib/services/scanner_service.dart`
- UI 展示：`lib/widgets/scan_result_view.dart`

---

## 2. 风险评估

### 功能描述

对扫描发现的资产进行安全风险评估。每个插件根据资产的配置情况（如端口暴露、权限设置、配置安全性等）生成相应的风险列表，风险分为 low、medium、high、critical 四个等级。部分风险提供自动修复表单或修复建议。

### 触发方式

资产扫描完成后自动触发风险评估，结果在扫描结果视图中展示。

### 代码依据
- FFI 入口：`go_lib/main.go` — `AssessRisksFFI`
- 风险模型：`go_lib/core/risk.go` — `Risk`、`Mitigation`
- 插件实现示例：`go_lib/plugins/openclaw/checker.go`
- UI 展示：`lib/widgets/scan_result_view.dart`
- Flutter 模型：`lib/models/risk_model.dart`

---

## 3. 风险缓解

### 功能描述

针对评估出的风险项，提供交互式修复能力。支持三种缓解类型：自动修复（auto）、表单交互修复（form）、建议指引（suggestion）。用户可通过弹出的对话框填写参数并执行修复操作。

### 触发方式

在扫描结果页面中，点击风险项的"修复"按钮，弹出修复对话框。

### 代码依据
- FFI 入口：`go_lib/main.go` — `MitigateRiskFFI`
- 插件实现：`go_lib/plugins/openclaw/mitigation.go`
- 缓解策略定义：`go_lib/plugins/openclaw/mitigation.json`
- UI 对话框：`lib/widgets/mitigation_dialog.dart`

---

## 4. 实时代理防护

### 功能描述

核心防护功能。通过在 Bot 与 LLM API 之间插入本地反向代理，实时拦截和分析所有 API 请求与响应。代理支持流式和非流式请求转发，集成 ShepherdGate 安全网关进行 tool_call 级别的风险分析。支持"仅审计"模式（只记录不拦截）和"主动防护"模式（检测到风险时拦截请求）。

### 触发方式

在主界面配置防护参数后，点击"启动防护"按钮。可配置代理端口、目标 LLM 地址、是否开启沙箱、审计模式等。

### 代码依据
- FFI 入口：`go_lib/main.go` — `StartProtectionProxy`、`StopProtectionProxy`、`GetProtectionProxyStatus`
- 代理核心：`go_lib/core/proxy/proxy_protection.go` — `ProxyProtection`
- 请求处理：`go_lib/core/proxy/proxy_protection_handler.go`
- 流式缓冲：`go_lib/core/proxy/stream_buffer.go`
- LLM 路由：`go_lib/chatmodel-routing/proxy.go` — `Proxy`
- 配置管理：`go_lib/core/plugin.go` — `ProtectionConfig`
- Flutter 服务：`lib/services/protection_service.dart`、`lib/services/protection_proxy_ffi.dart`
- UI 配置：`lib/widgets/protection_config_dialog.dart`

---

## 5. ShepherdGate 安全网关

### 功能描述

基于 LLM 的智能安全分析引擎，集成在代理防护层内。ShepherdGate 使用独立的安全模型对 Bot 的 tool_call 调用进行实时风险研判，输出允许（ALLOWED）或需确认（NEEDS_CONFIRMATION）的决策。支持 ReAct+Skill 范式的深度分析，可加载内置安全技能（如脚本执行检测、数据外泄检测等）进行专项分析。支持用户自定义安全规则。

### 触发方式

防护启动后自动生效。当代理检测到 Bot 发起 tool_call 时，自动调用 ShepherdGate 进行安全分析。用户可通过防护配置设置安全模型和自定义规则。

### 代码依据
- ShepherdGate 核心：`go_lib/core/shepherd/shepherd_gate.go` — `ShepherdGate`、`ShepherdDecision`
- ReAct 分析器：`go_lib/core/shepherd/shepherd_react_analyzer.go` — `ToolCallReActAnalyzer`
- 内置技能：`go_lib/core/shepherd/bundled_react_skills/`
- 用户规则：`go_lib/core/shepherd/shepherd_user_rules_bundle.go`
- FFI 入口：`go_lib/main.go` — `UpdateShepherdRulesFFI`、`GetShepherdRulesFFI`、`ListBundledReActSkillsFFI`
- 安全模型配置：`go_lib/core/service/security_model_config_service.go`

---

## 6. 工具调用验证

### 功能描述

在代理层对 Bot 的工具调用进行规则化预检。支持白名单模式（仅允许指定工具）、黑名单模式（阻止指定工具）和禁用模式。规则可包含工具名匹配（支持通配符）和参数正则匹配，用于快速过滤高风险工具调用。

### 触发方式

防护运行期间自动生效，根据配置的工具验证规则对每次 tool_call 进行检查。

### 代码依据
- 工具验证器：`go_lib/core/proxy/tool_validator.go` — `ToolValidator`、`ToolValidatorConfig`

---

## 7. 沙箱防护

### 功能描述

通过操作系统级沙箱机制限制 Bot Gateway 进程的权限。macOS 使用 `sandbox-exec`（Seatbelt 策略），Linux 使用 `LD_PRELOAD` + JSON 策略文件，Windows 使用 `sandbox_hook.dll` + MinHook 注入。沙箱可限制文件系统访问、网络访问和命令执行权限。支持进程监控，在 Gateway 异常退出时自动重启。

### 触发方式

在防护配置对话框中开启"沙箱防护"选项。启动防护时，系统自动生成沙箱策略并以沙箱模式启动 Gateway 进程。

### 代码依据
- FFI 入口：`go_lib/main.go` — `StartSandboxedGateway`、`StopSandboxedGateway`、`GetSandboxStatus`、`CheckSandboxSupported`、`GenerateSandboxPolicy`
- 沙箱管理器：`go_lib/core/sandbox/manager.go`
- 平台实现：`go_lib/core/sandbox/seatbelt.go`（macOS）、`go_lib/core/sandbox/preload_linux.go`（Linux）、`go_lib/core/sandbox/hook_windows.go`（Windows）
- 进程监控：`go_lib/core/sandbox/monitor.go`
- 策略配置：`go_lib/core/sandbox/config.go`
- Flutter 服务：`lib/services/sandbox_service.dart`

---

## 8. 技能安全扫描

### 功能描述

使用 AI 对 Bot 加载的技能（Skills）进行安全分析。分析维度包括：脚本执行风险、数据外泄风险、混淆/逃避风险、依赖供应链风险和社会工程陷阱风险。支持单个技能扫描和批量扫描，扫描结果包含安全评分和详细的风险分析报告。用户可对安全的技能标记为"已信任"。

### 触发方式

在主界面点击"技能扫描"按钮打开扫描对话框，选择技能后启动扫描。也可批量扫描所有已发现的技能。

### 代码依据
- FFI 入口：`go_lib/main.go` — `StartSkillSecurityScan`、`GetSkillSecurityScanResult`、`StartBatchSkillScan`
- 安全分析器：`go_lib/core/skillscan/skill_security_analyzer.go` — `SkillAnalysisResult`
- 分析提示词：`go_lib/core/skillscan/skill_security_analyzer_prompt.go`
- 内置扫描技能：`go_lib/core/skillscan/bundled_scan_skills/`
- 技能发现：`go_lib/core/skillscan/skill_discovery.go`
- 技能哈希：`go_lib/core/skillscan/skill_hash.go`
- Flutter 服务：`lib/services/skill_security_analyzer_service.dart`
- UI 对话框：`lib/widgets/skill_scan_dialog.dart`、`lib/widgets/skill_scan_results_dialog.dart`
- 数据持久化：`go_lib/core/service/scan_service.go` — `SaveSkillScanResult`、`TrustSkill`

---

## 9. 审计日志

### 功能描述

记录所有通过代理防护层的 API 请求的完整审计信息，包括请求内容、响应内容、使用的模型、风险判断结果、执行动作、Token 用量和耗时等。支持按资产、时间、风险类型等维度过滤查询，支持统计分析和定期清理。可在独立窗口中打开审计日志查看器。

### 触发方式

防护运行期间自动记录。用户可在主界面或独立窗口中查看审计日志。

### 代码依据
- FFI 入口：`go_lib/main.go` — `SaveAuditLogFFI`、`GetAuditLogsFFI`、`GetAuditLogStatisticsFFI`、`CleanOldAuditLogsFFI`、`ClearAuditLogsWithFilterFFI`
- TruthRecord（核心审计实体）：`go_lib/core/proxy/truth_record.go` — `TruthRecord`
- 审计日志服务：`go_lib/core/service/audit_log_service.go`
- Flutter 服务：`lib/services/audit_log_database_service.dart`
- 独立窗口：`lib/pages/audit_log_window.dart`

---

## 10. 安全事件

### 功能描述

实时捕获和记录防护过程中的安全事件，如检测到的风险 tool_call、被拦截的请求等。安全事件通过回调桥接实时推送到 Flutter UI，支持按资产过滤和按请求 ID 关联查询。

### 触发方式

防护运行期间自动触发。安全事件在防护监控窗口中实时展示。

### 代码依据
- FFI 入口：`go_lib/main.go` — `SaveSecurityEventsBatchFFI`、`GetSecurityEventsFFI`、`GetPendingSecurityEvents`
- 安全事件缓冲：`go_lib/core/shepherd/security_event.go`
- 安全事件服务：`go_lib/core/service/security_event_service.go`
- Flutter 服务：`lib/services/security_event_database_service.dart`

---

## 11. 防护监控

### 功能描述

提供专用的防护监控窗口，实时展示代理防护的运行状态、日志流、安全事件和统计图表。监控窗口可独立于主窗口打开，支持日志级别过滤、日志搜索、事件面板和图表统计面板。

### 触发方式

防护启动后，在主界面点击"防护监控"按钮打开独立监控窗口。

### 代码依据
- 独立窗口：`lib/pages/protection_monitor_window.dart`
- 日志面板：`lib/widgets/protection_monitor_log_panel.dart`
- 事件面板：`lib/widgets/protection_monitor_event_panel.dart`
- 图表组件：`lib/widgets/protection_monitor_charts.dart`
- 监控服务：`lib/services/protection_monitor_service.dart`

---

## 12. LLM 多厂商路由

### 功能描述

代理层支持多种 LLM 服务商的协议适配和请求转发。当前支持的 Provider 包括：OpenAI、Anthropic、Google、DeepSeek、Moonshot、Ollama、xAI。Provider 可在运行时热切换，无需重启代理。

### 触发方式

在防护配置中选择目标 LLM 服务商和模型，或通过模型配置对话框进行设置。

### 代码依据
- FFI 入口：`go_lib/main.go` — `GetSupportedProviders`、`UpdateBotForwardingProvider`
- Provider 接口：`go_lib/chatmodel-routing/adapter/provider.go` — `Provider`
- 各厂商实现：`go_lib/chatmodel-routing/providers/`（openai/anthropic/google/deepseek/moonshot/ollama/xai）
- 代理路由：`go_lib/chatmodel-routing/proxy.go` — `Proxy`
- Flutter 服务：`lib/services/provider_service.dart`

---

## 13. 模型配置管理

### 功能描述

管理两类模型配置：安全模型配置（供 ShepherdGate 使用的安全分析模型）和 Bot 模型配置（供 Bot 代理转发使用的上游模型）。支持模型连接测试，验证 API Key 和端点的可用性。

### 触发方式

通过设置对话框或防护配置对话框中的模型配置表单进行设置。连接测试通过表单中的"测试连接"按钮触发。

### 代码依据
- FFI 入口：`go_lib/main.go` — `SaveSecurityModelConfigFFI`、`GetSecurityModelConfigFFI`、`SaveBotModelConfigFFI`、`GetBotModelConfigFFI`、`TestModelConnectionFFI`
- 安全模型服务：`go_lib/core/service/security_model_config_service.go`
- Bot 模型服务：`go_lib/core/service/bot_model_config_service.go`
- Flutter 服务：`lib/services/model_config_database_service.dart`、`lib/services/model_config_service.dart`
- UI 表单：`lib/widgets/security_model_config_form.dart`、`lib/widgets/bot_model_config_form.dart`

---

## 14. API 指标统计

### 功能描述

收集和统计代理防护期间的 API 调用指标，包括请求次数、Token 用量（每日统计）、响应时间等。支持按资产维度查询统计数据，提供历史指标的定期清理能力。

### 触发方式

防护运行期间自动采集。统计数据在防护监控窗口的图表面板中展示。

### 代码依据
- FFI 入口：`go_lib/main.go` — `SaveApiMetricsFFI`、`GetApiStatisticsFFI`、`GetDailyTokenUsageFFI`、`CleanOldApiMetricsFFI`
- 指标服务：`go_lib/core/service/metrics_service.go`
- Flutter 服务：`lib/services/metrics_database_service.dart`

---

## 15. 插件系统

### 功能描述

采用插件化架构，每种 Bot 类型由独立插件实现。插件需实现 `BotPlugin` 接口，提供资产发现、风险评估、防护控制等核心方法。当前内置三个插件：Openclaw（通用 MCP Bot）、Nullclaw 和 DinTalClaw（政务龙虾）。插件通过 `init()` 函数自注册到 `PluginManager`，支持可选能力扩展（技能扫描、模型连接测试、网关沙箱同步、应用退出生命周期等）。

### 触发方式

插件在应用启动时自动加载和注册。用户可通过 `GetPluginsFFI` 获取已注册插件列表。

### 代码依据
- FFI 入口：`go_lib/main.go` — `GetPluginsFFI`
- 插件接口：`go_lib/core/plugin.go` — `BotPlugin`
- 插件管理器：`go_lib/core/plugin_manager.go` — `PluginManager`
- 可选能力：`go_lib/core/plugin_capabilities.go` — `SkillScanCapability`、`ModelConnectionCapability`、`GatewaySandboxCapability` 等
- Openclaw 插件：`go_lib/plugins/openclaw/plugin.go`
- Nullclaw 插件：`go_lib/plugins/nullclaw/plugin.go`
- DinTalClaw 插件：`go_lib/plugins/dintalclaw/plugin.go`
- Flutter 服务：`lib/services/plugin_service.dart`

---

## 16. 回调桥接通信

### 功能描述

实现 Go 到 Flutter 的实时消息推送通道。通过 FFI Callback 机制，Go 端可主动向 Dart 端推送日志、安全事件、版本更新通知等消息，无需 Dart 端轮询。

### 触发方式

应用启动时自动注册回调。通过 `RegisterMessageCallback` FFI 建立通道。

### 代码依据
- FFI 入口：`go_lib/main.go` — `RegisterMessageCallback`、`UnregisterMessageCallback`、`IsCallbackBridgeRunning`
- 回调桥接：`go_lib/core/callback_bridge/`
- Flutter 服务：`lib/services/message_bridge_service.dart`

---

## 17. 版本检查

### 功能描述

后台定期检查应用新版本。首次检查在启动后 60 秒延迟执行，之后每 4 小时检查一次。发现新版本后通过回调桥接通知 Flutter UI，展示更新信息（版本号、更新日志、下载地址等）。

### 触发方式

应用启动后自动开始。通过设置可启用或禁用。

### 代码依据
- FFI 入口：`go_lib/main.go` — `StartVersionCheckServiceFFI`、`StopVersionCheckServiceFFI`
- 版本检查服务：`go_lib/core/service/version_check_service.go` — `VersionCheckService`
- Flutter Mixin：`lib/pages/mixins/main_page_version_mixin.dart`

---

## 18. 配置备份与恢复

### 功能描述

支持 Bot 配置的初始备份和恢复。在首次启动防护时自动创建配置备份，用户可随时将配置恢复到初始状态。应用退出时也可自动恢复 Bot 的默认状态。

### 触发方式

首次启动防护时自动备份。通过设置中的"恢复初始配置"选项手动恢复。

### 代码依据
- FFI 入口：`go_lib/main.go` — `HasInitialBackupFFI`、`RestoreToInitialConfigFFI`、`NotifyPluginAppExitFFI`、`RestoreBotDefaultStateFFI`
- 能力接口：`go_lib/core/plugin_capabilities.go` — `GatewaySandboxCapability`、`ApplicationLifecycleCapability`
- 插件实现：`go_lib/plugins/openclaw/openclaw_config_updater.go`

---

## 19. 应用设置

### 功能描述

提供全局应用设置管理，包括语言切换（中文/英文）、开机自启动、定时扫描间隔、通用键值对设置存储等。语言切换会同步更新运行时的 ShepherdGate 语言。

### 触发方式

点击主界面的设置图标打开设置对话框。

### 代码依据
- FFI 入口：`go_lib/main.go` — `SetLanguageFFI`、`GetLanguageFFI`、`SaveAppSettingFFI`、`GetAppSettingFFI`
- 设置服务：`go_lib/core/service/app_settings_service.go`
- Flutter 服务：`lib/services/app_settings_database_service.dart`
- UI 对话框：`lib/widgets/settings_dialog.dart`、`lib/widgets/general_settings_tab.dart`

---

## 20. 新手引导

### 功能描述

首次使用时展示欢迎页面和交互式引导流程，帮助用户完成初始配置（如安全模型设置、Bot 模型设置等）。引导完成后展示完成动画覆盖层。

### 触发方式

首次启动应用时自动触发。

### 代码依据
- UI 组件：`lib/widgets/welcome_overlay.dart`、`lib/widgets/onboarding_dialog.dart`、`lib/widgets/onboarding_completion_overlay.dart`
- 引导服务：`lib/services/onboarding_service.dart`

---

## 21. 技能代理框架

### 功能描述

提供基于 Eino ADK 的技能代理（Skill Agent）框架，支持以 ReAct 范式加载和执行技能。技能代理用于 ShepherdGate 的安全分析和技能安全扫描中的深度分析任务。

### 触发方式

由 ShepherdGate 和技能安全扫描器内部调用，用户无需直接操作。

### 代码依据
- 技能代理：`go_lib/skillagent/agent.go`
- 技能定义：`go_lib/skillagent/skill.go`
- 流式处理：`go_lib/skillagent/stream.go`
- ADK 适配：`go_lib/skillagent/adk_adapter.go`

---

## 22. 系统托盘

### 功能描述

支持最小化到系统托盘运行。提供托盘图标和右键菜单，用户可从托盘快速显示/隐藏主窗口。

### 触发方式

关闭或最小化主窗口时自动缩至托盘。macOS 支持 Cmd+W 快捷键隐藏窗口。

### 代码依据
- 托盘 Mixin：`lib/pages/mixins/main_page_tray_mixin.dart`
- 窗口 Mixin：`lib/pages/mixins/main_page_window_mixin.dart`
- 快捷键：`lib/widgets/hide_window_shortcut.dart`
