# ClawSecbot Product Features Guide

## Overview

ClawSecbot is a desktop security management tool for AI Bots (such as Openclaw, Nullclaw, DinTalClaw, etc.). Built on a Flutter Desktop + Go + FFI architecture, it provides asset auto-discovery, security risk assessment, real-time proxy protection, sandbox isolation, skill security scanning, audit logging and more, helping users comprehensively manage locally running AI Bot instances.

---

## 1. Asset Scanning & Discovery

### Description

Automatically scans the local system to discover running AI Bot instances. The scanner collects system snapshots (processes, ports, configuration files, etc.) and matches predefined detection rules to identify different types of Bot assets. Each discovered asset is assigned a unique `asset_id` fingerprint based on its name, config path, ports and process paths.

### How to Trigger

Triggered by clicking the scan button on the main page. Results are displayed on the main page after scanning. Periodic automatic scanning can also be configured with a scheduled scan interval.

### Code References
- FFI entry: `go_lib/main.go` — `ScanAssetsFFI`
- Scanner core: `go_lib/core/scanner/scanner.go` — `AssetScanner.Scan()`
- System collectors: `go_lib/core/collector_darwin.go`, `collector_linux.go`, `collector_windows.go`
- Rule engine: `go_lib/core/engine.go` — `AssetDetectionEngine`
- Asset model: `go_lib/core/asset.go` — `Asset`, `ComputeAssetID()`
- Flutter scanner service: `lib/services/scanner_service.dart`
- UI display: `lib/widgets/scan_result_view.dart`

---

## 2. Risk Assessment

### Description

Performs security risk assessment on discovered assets. Each plugin evaluates asset configurations (such as port exposure, permission settings, configuration security, etc.) and generates a risk list with four severity levels: low, medium, high, and critical. Some risks provide automated remediation forms or remediation suggestions.

### How to Trigger

Risk assessment is automatically triggered after asset scanning completes. Results are displayed in the scan result view.

### Code References
- FFI entry: `go_lib/main.go` — `AssessRisksFFI`
- Risk model: `go_lib/core/risk.go` — `Risk`, `Mitigation`
- Plugin implementation example: `go_lib/plugins/openclaw/checker.go`
- UI display: `lib/widgets/scan_result_view.dart`
- Flutter model: `lib/models/risk_model.dart`

---

## 3. Risk Mitigation

### Description

Provides interactive remediation for identified risks. Three mitigation types are supported: automatic fix (auto), interactive form-based fix (form), and suggestion guidance (suggestion). Users can fill in parameters and execute remediation through popup dialogs.

### How to Trigger

On the scan result page, click the "Fix" button on a risk item to open the remediation dialog.

### Code References
- FFI entry: `go_lib/main.go` — `MitigateRiskFFI`
- Plugin implementation: `go_lib/plugins/openclaw/mitigation.go`
- Mitigation strategy definition: `go_lib/plugins/openclaw/mitigation.json`
- UI dialog: `lib/widgets/mitigation_dialog.dart`

---

## 4. Real-time Proxy Protection

### Description

The core protection feature. It inserts a local reverse proxy between the Bot and the LLM API to intercept and analyze all API requests and responses in real time. The proxy supports both streaming and non-streaming request forwarding, integrating the ShepherdGate security gateway for tool_call-level risk analysis. Supports "audit-only" mode (logging without blocking) and "active protection" mode (blocking requests when risks are detected).

### How to Trigger

Configure protection parameters on the main page and click the "Start Protection" button. Configurable options include proxy port, target LLM address, sandbox toggle, audit mode, etc.

### Code References
- FFI entry: `go_lib/main.go` — `StartProtectionProxy`, `StopProtectionProxy`, `GetProtectionProxyStatus`
- Proxy core: `go_lib/core/proxy/proxy_protection.go` — `ProxyProtection`
- Request handler: `go_lib/core/proxy/proxy_protection_handler.go`
- Stream buffer: `go_lib/core/proxy/stream_buffer.go`
- LLM routing: `go_lib/chatmodel-routing/proxy.go` — `Proxy`
- Configuration: `go_lib/core/plugin.go` — `ProtectionConfig`
- Flutter services: `lib/services/protection_service.dart`, `lib/services/protection_proxy_ffi.dart`
- UI configuration: `lib/widgets/protection_config_dialog.dart`

---

## 5. ShepherdGate Security Gateway

### Description

An LLM-powered intelligent security analysis engine embedded in the proxy protection layer. ShepherdGate uses a dedicated security model to perform real-time risk assessment on Bot tool_call invocations, producing either ALLOWED or NEEDS_CONFIRMATION decisions. It supports the ReAct+Skill paradigm for deep analysis, loading built-in security skills (such as script execution detection, data exfiltration detection, etc.) for specialized analysis. Custom user security rules are also supported.

### How to Trigger

Automatically active when protection is started. When the proxy detects a tool_call from the Bot, it automatically invokes ShepherdGate for security analysis. Users can configure the security model and custom rules through protection settings.

### Code References
- ShepherdGate core: `go_lib/core/shepherd/shepherd_gate.go` — `ShepherdGate`, `ShepherdDecision`
- ReAct analyzer: `go_lib/core/shepherd/shepherd_react_analyzer.go` — `ToolCallReActAnalyzer`
- Built-in skills: `go_lib/core/shepherd/bundled_react_skills/`
- User rules: `go_lib/core/shepherd/shepherd_user_rules_bundle.go`
- FFI entry: `go_lib/main.go` — `UpdateShepherdRulesFFI`, `GetShepherdRulesFFI`, `ListBundledReActSkillsFFI`
- Security model config: `go_lib/core/service/security_model_config_service.go`

---

## 6. Tool Call Validation

### Description

Rule-based pre-validation of Bot tool calls at the proxy layer. Supports whitelist mode (allow only specified tools), blacklist mode (block specified tools), and disabled mode. Rules can include tool name matching (with wildcards) and argument regex matching for fast filtering of high-risk tool calls.

### How to Trigger

Automatically active during protection, checking each tool_call against configured tool validation rules.

### Code References
- Tool validator: `go_lib/core/proxy/tool_validator.go` — `ToolValidator`, `ToolValidatorConfig`

---

## 7. Sandbox Protection

### Description

Restricts Bot Gateway process permissions through OS-level sandbox mechanisms. macOS uses `sandbox-exec` (Seatbelt policy), Linux uses `LD_PRELOAD` + JSON policy files, and Windows uses `sandbox_hook.dll` + MinHook injection. The sandbox can restrict filesystem access, network access, and command execution permissions. Process monitoring is supported to automatically restart the Gateway on unexpected exits.

### How to Trigger

Enable the "Sandbox Protection" option in the protection configuration dialog. When starting protection, the system automatically generates sandbox policies and launches the Gateway process in sandboxed mode.

### Code References
- FFI entry: `go_lib/main.go` — `StartSandboxedGateway`, `StopSandboxedGateway`, `GetSandboxStatus`, `CheckSandboxSupported`, `GenerateSandboxPolicy`
- Sandbox manager: `go_lib/core/sandbox/manager.go`
- Platform implementations: `go_lib/core/sandbox/seatbelt.go` (macOS), `go_lib/core/sandbox/preload_linux.go` (Linux), `go_lib/core/sandbox/hook_windows.go` (Windows)
- Process monitor: `go_lib/core/sandbox/monitor.go`
- Policy configuration: `go_lib/core/sandbox/config.go`
- Flutter service: `lib/services/sandbox_service.dart`

---

## 8. Skill Security Scanning

### Description

Uses AI to perform security analysis on Bot-loaded Skills. Analysis dimensions include: script execution risk, data exfiltration risk, obfuscation/evasion risk, dependency supply chain risk, and social engineering trap risk. Supports single skill scanning and batch scanning, with results including safety scores and detailed risk analysis reports. Users can mark safe skills as "trusted".

### How to Trigger

Click the "Skill Scan" button on the main page to open the scan dialog, select a skill and start scanning. Batch scanning of all discovered skills is also available.

### Code References
- FFI entry: `go_lib/main.go` — `StartSkillSecurityScan`, `GetSkillSecurityScanResult`, `StartBatchSkillScan`
- Security analyzer: `go_lib/core/skillscan/skill_security_analyzer.go` — `SkillAnalysisResult`
- Analysis prompts: `go_lib/core/skillscan/skill_security_analyzer_prompt.go`
- Built-in scan skills: `go_lib/core/skillscan/bundled_scan_skills/`
- Skill discovery: `go_lib/core/skillscan/skill_discovery.go`
- Skill hash: `go_lib/core/skillscan/skill_hash.go`
- Flutter service: `lib/services/skill_security_analyzer_service.dart`
- UI dialogs: `lib/widgets/skill_scan_dialog.dart`, `lib/widgets/skill_scan_results_dialog.dart`
- Data persistence: `go_lib/core/service/scan_service.go` — `SaveSkillScanResult`, `TrustSkill`

---

## 9. Audit Logging

### Description

Records complete audit information for all API requests passing through the proxy protection layer, including request content, response content, model used, risk judgment results, actions taken, token usage, and latency. Supports filtering by asset, time, risk type, and other dimensions, with statistical analysis and periodic cleanup. Can be opened in a separate window as an audit log viewer.

### How to Trigger

Automatically recorded during protection runtime. Users can view audit logs from the main page or in a separate window.

### Code References
- FFI entry: `go_lib/main.go` — `SaveAuditLogFFI`, `GetAuditLogsFFI`, `GetAuditLogStatisticsFFI`, `CleanOldAuditLogsFFI`, `ClearAuditLogsWithFilterFFI`
- TruthRecord (core audit entity): `go_lib/core/proxy/truth_record.go` — `TruthRecord`
- Audit log service: `go_lib/core/service/audit_log_service.go`
- Flutter service: `lib/services/audit_log_database_service.dart`
- Separate window: `lib/pages/audit_log_window.dart`

---

## 10. Security Events

### Description

Captures and records security events in real time during protection, such as detected risky tool_calls and blocked requests. Security events are pushed to the Flutter UI in real time via callback bridge, with support for filtering by asset and querying by request ID.

### How to Trigger

Automatically triggered during protection runtime. Security events are displayed in real time in the protection monitor window.

### Code References
- FFI entry: `go_lib/main.go` — `SaveSecurityEventsBatchFFI`, `GetSecurityEventsFFI`, `GetPendingSecurityEvents`
- Security event buffer: `go_lib/core/shepherd/security_event.go`
- Security event service: `go_lib/core/service/security_event_service.go`
- Flutter service: `lib/services/security_event_database_service.dart`

---

## 11. Protection Monitoring

### Description

Provides a dedicated protection monitoring window that displays proxy protection runtime status, log streams, security events, and statistical charts in real time. The monitor window can be opened independently from the main window, with support for log level filtering, log searching, event panel, and chart statistics panel.

### How to Trigger

After protection starts, click the "Protection Monitor" button on the main page to open an independent monitor window.

### Code References
- Separate window: `lib/pages/protection_monitor_window.dart`
- Log panel: `lib/widgets/protection_monitor_log_panel.dart`
- Event panel: `lib/widgets/protection_monitor_event_panel.dart`
- Chart components: `lib/widgets/protection_monitor_charts.dart`
- Monitor service: `lib/services/protection_monitor_service.dart`

---

## 12. LLM Provider Multi-vendor Routing

### Description

The proxy layer supports protocol adaptation and request forwarding for multiple LLM service providers. Currently supported providers include: OpenAI, Anthropic, Google, DeepSeek, Moonshot, Ollama, and xAI. Providers can be hot-swapped at runtime without restarting the proxy.

### How to Trigger

Select the target LLM provider and model in the protection configuration, or configure through the model configuration dialog.

### Code References
- FFI entry: `go_lib/main.go` — `GetSupportedProviders`, `UpdateBotForwardingProvider`
- Provider interface: `go_lib/chatmodel-routing/adapter/provider.go` — `Provider`
- Provider implementations: `go_lib/chatmodel-routing/providers/` (openai/anthropic/google/deepseek/moonshot/ollama/xai)
- Proxy routing: `go_lib/chatmodel-routing/proxy.go` — `Proxy`
- Flutter service: `lib/services/provider_service.dart`

---

## 13. Model Configuration Management

### Description

Manages two types of model configurations: security model config (for the ShepherdGate security analysis model) and Bot model config (for Bot proxy forwarding to upstream models). Supports model connection testing to verify API Key and endpoint availability.

### How to Trigger

Configured through the settings dialog or model configuration forms in the protection configuration dialog. Connection testing is triggered via the "Test Connection" button in the form.

### Code References
- FFI entry: `go_lib/main.go` — `SaveSecurityModelConfigFFI`, `GetSecurityModelConfigFFI`, `SaveBotModelConfigFFI`, `GetBotModelConfigFFI`, `TestModelConnectionFFI`
- Security model service: `go_lib/core/service/security_model_config_service.go`
- Bot model service: `go_lib/core/service/bot_model_config_service.go`
- Flutter services: `lib/services/model_config_database_service.dart`, `lib/services/model_config_service.dart`
- UI forms: `lib/widgets/security_model_config_form.dart`, `lib/widgets/bot_model_config_form.dart`

---

## 14. API Metrics & Statistics

### Description

Collects and tracks API call metrics during proxy protection, including request counts, token usage (daily statistics), and response times. Supports querying statistics by asset dimension and provides periodic cleanup of historical metrics.

### How to Trigger

Automatically collected during protection runtime. Statistics are displayed in the chart panel of the protection monitor window.

### Code References
- FFI entry: `go_lib/main.go` — `SaveApiMetricsFFI`, `GetApiStatisticsFFI`, `GetDailyTokenUsageFFI`, `CleanOldApiMetricsFFI`
- Metrics service: `go_lib/core/service/metrics_service.go`
- Flutter service: `lib/services/metrics_database_service.dart`

---

## 15. Plugin System

### Description

Employs a plugin-based architecture where each Bot type is implemented by an independent plugin. Plugins must implement the `BotPlugin` interface, providing core methods for asset discovery, risk assessment, and protection control. Three built-in plugins are currently available: Openclaw (general MCP Bot), Nullclaw, and DinTalClaw. Plugins self-register to `PluginManager` via `init()` functions and support optional capability extensions (skill scanning, model connection testing, gateway sandbox sync, application exit lifecycle, etc.).

### How to Trigger

Plugins are automatically loaded and registered at application startup. Users can retrieve the registered plugin list via `GetPluginsFFI`.

### Code References
- FFI entry: `go_lib/main.go` — `GetPluginsFFI`
- Plugin interface: `go_lib/core/plugin.go` — `BotPlugin`
- Plugin manager: `go_lib/core/plugin_manager.go` — `PluginManager`
- Optional capabilities: `go_lib/core/plugin_capabilities.go` — `SkillScanCapability`, `ModelConnectionCapability`, `GatewaySandboxCapability`, etc.
- Openclaw plugin: `go_lib/plugins/openclaw/plugin.go`
- Nullclaw plugin: `go_lib/plugins/nullclaw/plugin.go`
- DinTalClaw plugin: `go_lib/plugins/dintalclaw/plugin.go`
- Flutter service: `lib/services/plugin_service.dart`

---

## 16. Callback Bridge Communication

### Description

Implements a real-time message push channel from Go to Flutter. Through the FFI Callback mechanism, the Go side can actively push logs, security events, version update notifications and other messages to the Dart side without polling.

### How to Trigger

Callbacks are automatically registered at application startup. The channel is established via `RegisterMessageCallback` FFI.

### Code References
- FFI entry: `go_lib/main.go` — `RegisterMessageCallback`, `UnregisterMessageCallback`, `IsCallbackBridgeRunning`
- Callback bridge: `go_lib/core/callback_bridge/`
- Flutter service: `lib/services/message_bridge_service.dart`

---

## 17. Version Check

### Description

Periodically checks for new application versions in the background. The first check runs after a 60-second startup delay, then every 4 hours thereafter. When a new version is found, the Flutter UI is notified via callback bridge, displaying update information (version number, changelog, download URL, etc.).

### How to Trigger

Automatically starts after application launch. Can be enabled or disabled through settings.

### Code References
- FFI entry: `go_lib/main.go` — `StartVersionCheckServiceFFI`, `StopVersionCheckServiceFFI`
- Version check service: `go_lib/core/service/version_check_service.go` — `VersionCheckService`
- Flutter mixin: `lib/pages/mixins/main_page_version_mixin.dart`

---

## 18. Configuration Backup & Restore

### Description

Supports initial backup and restoration of Bot configurations. An automatic configuration backup is created when protection starts for the first time, and users can restore the configuration to its initial state at any time. Bot default state can also be automatically restored on application exit.

### How to Trigger

Automatically backed up when protection is first started. Manually restored through the "Restore Initial Config" option in settings.

### Code References
- FFI entry: `go_lib/main.go` — `HasInitialBackupFFI`, `RestoreToInitialConfigFFI`, `NotifyPluginAppExitFFI`, `RestoreBotDefaultStateFFI`
- Capability interface: `go_lib/core/plugin_capabilities.go` — `GatewaySandboxCapability`, `ApplicationLifecycleCapability`
- Plugin implementation: `go_lib/plugins/openclaw/openclaw_config_updater.go`

---

## 19. Application Settings

### Description

Provides global application settings management, including language switching (Chinese/English), launch at startup, scheduled scan interval, and general key-value settings storage. Language changes also synchronously update the ShepherdGate runtime language.

### How to Trigger

Click the settings icon on the main page to open the settings dialog.

### Code References
- FFI entry: `go_lib/main.go` — `SetLanguageFFI`, `GetLanguageFFI`, `SaveAppSettingFFI`, `GetAppSettingFFI`
- Settings service: `go_lib/core/service/app_settings_service.go`
- Flutter service: `lib/services/app_settings_database_service.dart`
- UI dialogs: `lib/widgets/settings_dialog.dart`, `lib/widgets/general_settings_tab.dart`

---

## 20. Onboarding

### Description

Displays a welcome page and interactive onboarding flow on first use, guiding users through initial configuration (such as security model setup, Bot model setup, etc.). A completion animation overlay is shown after onboarding finishes.

### How to Trigger

Automatically triggered on first application launch.

### Code References
- UI components: `lib/widgets/welcome_overlay.dart`, `lib/widgets/onboarding_dialog.dart`, `lib/widgets/onboarding_completion_overlay.dart`
- Onboarding service: `lib/services/onboarding_service.dart`

---

## 21. Skill Agent Framework

### Description

Provides an Eino ADK-based Skill Agent framework that supports loading and executing skills in the ReAct paradigm. The skill agent is used for deep analysis tasks in ShepherdGate security analysis and skill security scanning.

### How to Trigger

Internally invoked by ShepherdGate and the skill security scanner; no direct user interaction required.

### Code References
- Skill agent: `go_lib/skillagent/agent.go`
- Skill definition: `go_lib/skillagent/skill.go`
- Stream processing: `go_lib/skillagent/stream.go`
- ADK adapter: `go_lib/skillagent/adk_adapter.go`

---

## 22. System Tray

### Description

Supports minimizing to system tray. Provides a tray icon and right-click menu for users to quickly show/hide the main window from the tray.

### How to Trigger

Automatically minimized to tray when the main window is closed or minimized. macOS supports Cmd+W shortcut to hide the window.

### Code References
- Tray mixin: `lib/pages/mixins/main_page_tray_mixin.dart`
- Window mixin: `lib/pages/mixins/main_page_window_mixin.dart`
- Shortcut: `lib/widgets/hide_window_shortcut.dart`
