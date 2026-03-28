# ClawSecbot 配置系统架构

本文档描述 ClawSecbot 中安全模型配置、防护配置、系统设置等所有设置信息的代码结构和逻辑。

---

## 1. 配置体系总览

ClawSecbot 的配置体系分为以下几个层次：

| 配置类型 | 作用域 | 存储位置 | 主要用途 |
|---------|-------|---------|---------|
| SecurityModelConfig | 全局唯一 | SQLite `security_model_config` 表 | ShepherdGate 风险检测 |
| BotModelConfig | 按资产 | SQLite `protection_configs` 表 | 代理转发目标 |
| ProtectionConfig | 按资产 | SQLite `protection_configs` 表 | 防护运行时配置 |
| AppSettings | 全局 | SQLite `app_settings` 表 | 应用级设置（语言等） |
| ShepherdGate UserRules | 运行时 | 内存（Go 全局变量） | 敏感操作规则 |

---

## 2. SecurityModelConfig（安全模型配置）

### 2.1 职责定义

SecurityModelConfig 专门用于 **ShepherdGate 风险检测**，配置安全审计使用的 LLM 模型。与 BotModelConfig 完全独立，互不影响。

### 2.2 数据结构

**Flutter 端** - [lib/models/llm_config_model.dart](lib/models/llm_config_model.dart)
```dart
class SecurityModelConfig {
  final String provider;   // LLM 提供商: 'openai', 'anthropic', 'ollama', 'deepseek' 等
  final String endpoint;   // API 端点或基础 URL
  final String apiKey;     // API 密钥
  final String model;      // 模型名称: 'gpt-4', 'claude-3.5-sonnet' 等
  final String secretKey;  // 特定提供商需要（如千帆）
}
```

**Go 端** - [go_lib/plugins/openclaw/model_config.go](go_lib/plugins/openclaw/model_config.go:37-48)
```go
type SecurityModelConfig struct {
    Provider  ModelType `json:"provider"`
    Endpoint  string    `json:"endpoint"`
    APIKey    string    `json:"api_key"`
    Model     string    `json:"model"`
    SecretKey string    `json:"secret_key,omitempty"`
}
```

### 2.3 支持的 Provider 类型

```go
const (
    ModelTypeOpenAI   ModelType = "openai"
    ModelTypeOllama   ModelType = "ollama"
    ModelTypeDeepSeek ModelType = "deepseek"
    ModelTypeClaude   ModelType = "claude"
    ModelTypeGemini   ModelType = "gemini"
    ModelTypeGoogle   ModelType = "google"
    ModelTypeQianfan  ModelType = "qianfan"
    ModelTypeARK      ModelType = "ark"
)
```

### 2.4 数据流

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Flutter UI (SecurityModelConfigForm)                                    │
│   └─> SecurityModelConfigService.saveConfig()                           │
│         └─> ModelConfigDatabaseService.saveSecurityModelConfig()        │
│               └─> FFI: SaveSecurityModelConfigFFI                       │
│                     └─> Go: service.SaveSecurityModelConfig()           │
│                           └─> repository.SaveSecurityModelConfig()      │
│                                 └─> SQLite: model_config 表             │
└─────────────────────────────────────────────────────────────────────────┘
```

### 2.5 热更新机制

安全模型配置支持热更新，无需重启代理：

```dart
// Flutter 端
await protectionService.updateSecurityModelConfig(config);
```

```go
// Go 端 - ProxyProtection.UpdateSecurityModelConfig()
func (pp *ProxyProtection) UpdateSecurityModelConfig(config *SecurityModelConfig) error {
    return pp.shepherdGate.UpdateModelConfig(config)
}
```

### 2.6 数据库表结构

```sql
CREATE TABLE security_model_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),  -- 全局唯一
    provider TEXT,
    endpoint TEXT,
    api_key TEXT,
    model TEXT,
    secret_key TEXT,
    updated_at TEXT NOT NULL
);
```

---

## 3. BotModelConfig（Bot 模型配置）

### 3.1 职责定义

BotModelConfig 用于配置 **代理转发的目标 LLM**，即 Bot（如 Openclaw）实际调用的大模型服务。

### 3.2 数据结构

**Flutter 端** - [lib/models/llm_config_model.dart](lib/models/llm_config_model.dart:99-118)
```dart
class BotModelConfig {
  final String assetName;  // 关联的资产名称
  final String provider;   // LLM 提供商类型
  final String baseUrl;    // API 基础 URL
  final String apiKey;     // API 密钥
  final String model;      // 模型名称
  final String secretKey;  // 特定提供商需要
}
```

**Go 端** - [go_lib/plugins/openclaw/model_config.go](go_lib/plugins/openclaw/model_config.go:50-62)
```go
type BotModelConfig struct {
    Provider  string `json:"provider,omitempty"`
    BaseURL   string `json:"base_url,omitempty"`
    APIKey    string `json:"api_key,omitempty"`
    Model     string `json:"model,omitempty"`
    SecretKey string `json:"secret_key,omitempty"`
}
```

### 3.3 Provider 路由机制

Bot 模型的 Provider 从 UI 配置到 Go 层实例化的完整流程：

```
┌─────────────────────────────────────────────────────────────────────────┐
│ 1. UI 配置阶段                                                           │
│    BotModelConfigForm 用户选择 Provider（如 "openai"、"anthropic"）       │
│    └─> BotModelConfigService.saveConfig()                               │
│          └─> 存入 SQLite: protection_configs.bot_model_config (JSON)    │
│                                                                         │
│ 2. 启动代理阶段                                                          │
│    ProtectionService.startProtectionProxy()                             │
│    └─> _loadBotModelConfig() 从数据库读取 BotModelConfig                 │
│          └─> 构建 JSON payload: {"bot_model": {"provider": "openai"}}   │
│                └─> FFI 调用 StartProtectionProxyFFI                     │
│                                                                         │
│ 3. Go 层处理阶段                                                         │
│    NewProxyProtectionFromConfig() 解析 JSON                             │
│    └─> botModel.Provider 获取原始值 "openai"                             │
│          └─> adapter.NormalizeProviderName() 标准化为 ProviderName 枚举  │
│                └─> createForwardingProvider() 根据类型创建转发实例       │
└─────────────────────────────────────────────────────────────────────────┘
```

**关键函数**

| 函数 | 位置 | 作用 |
|------|------|------|
| `NormalizeProviderName()` | chatmodel-routing/adapter | 将字符串标准化为 ProviderName 枚举 |
| `createForwardingProvider()` | plugins/openclaw/proxy_protection.go | 根据 Provider 类型创建转发实例 |

### 3.4 与 openclaw.json 的关系

代理启动时，Go 层自动更新 openclaw.json 配置：

```go
// go_lib/plugins/openclaw/openclaw_config_updater.go
func ensureProviderForBotModel(rawConfig, botConfig, providerName, baseModel) {
    // 更新 models.providers 配置
    // 更新 primaryModel 引用
}
```

---

## 4. ProtectionConfig（防护配置）

### 4.1 职责定义

ProtectionConfig 是**按资产存储**的防护配置，包含：
- 防护启用状态
- 审计模式设置
- Token 限制
- 沙箱配置
- 权限设置
- Bot 模型配置引用

### 4.2 数据结构

**Flutter 端** - [lib/models/protection_config_model.dart](lib/models/protection_config_model.dart:1-70)
```dart
class ProtectionConfig {
  final String assetName;                    // 资产名称
  final bool enabled;                        // 防护启用
  final bool auditOnly;                      // 仅审计模式
  final bool sandboxEnabled;                 // 沙箱启用
  final String? gatewayBinaryPath;           // 网关二进制路径
  final String? gatewayConfigPath;           // 网关配置文件路径
  final int singleSessionTokenLimit;         // 单会话 Token 限制
  final int dailyTokenLimit;                 // 每日 Token 限制
  final PathPermissionConfig pathPermission;         // 路径权限
  final NetworkPermissionConfig networkPermission;   // 网络权限
  final ShellPermissionConfig shellPermission;       // Shell 权限
  final BotModelConfig? botModelConfig;      // Bot 模型配置
}
```

**Go 端 (Repository)** - [go_lib/core/repository/protection_repository.go](go_lib/core/repository/protection_repository.go:34-49)
```go
type ProtectionConfig struct {
    AssetName               string
    Enabled                 bool
    AuditOnly               bool
    SandboxEnabled          bool
    GatewayBinaryPath       string
    GatewayConfigPath       string
    SingleSessionTokenLimit int
    DailyTokenLimit         int
    PathPermission          string  // JSON 字符串
    NetworkPermission       string  // JSON 字符串
    ShellPermission         string  // JSON 字符串
    BotModelConfig          *BotModelConfigData
}
```

### 4.3 运行时配置结构

**Go 端 (Runtime)** - [go_lib/plugins/openclaw/model_config.go](go_lib/plugins/openclaw/model_config.go:70-83)
```go
type ProtectionRuntimeConfig struct {
    ProxyPort               int    `json:"proxy_port,omitempty"`
    AuditOnly               bool   `json:"audit_only,omitempty"`
    SingleSessionTokenLimit int    `json:"single_session_token_limit,omitempty"`
    DailyTokenLimit         int    `json:"daily_token_limit,omitempty"`
    InitialDailyTokenUsage  int    `json:"initial_daily_token_usage,omitempty"`
}
```

### 4.4 数据库表结构

```sql
CREATE TABLE protection_configs (
    asset_name TEXT PRIMARY KEY,
    enabled INTEGER NOT NULL DEFAULT 0,
    audit_only INTEGER NOT NULL DEFAULT 0,
    sandbox_enabled INTEGER NOT NULL DEFAULT 0,
    gateway_binary_path TEXT,
    gateway_config_path TEXT,
    custom_security_prompt TEXT,  -- 已废弃
    single_session_token_limit INTEGER DEFAULT 0,
    daily_token_limit INTEGER DEFAULT 0,
    path_permission TEXT,         -- JSON 字符串
    network_permission TEXT,      -- JSON 字符串
    shell_permission TEXT,        -- JSON 字符串
    bot_model_config TEXT,        -- JSON 字符串
    created_at TEXT,
    updated_at TEXT
);
```

---

## 5. 权限设置体系

### 5.1 权限模式

```dart
enum PermissionMode {
  whitelist,  // 白名单：仅允许列表中的项
  blacklist,  // 黑名单：禁止列表中的项
}
```

### 5.2 PathPermissionConfig（路径权限）

**Flutter 端** - [lib/models/protection_config_model.dart](lib/models/protection_config_model.dart:245-287)
```dart
class PathPermissionConfig {
  final PermissionMode mode;
  final List<String> paths;
  
  bool isPathAllowed(String path) {
    if (paths.isEmpty) return true;
    final isInList = paths.any((p) => path.startsWith(p) || p == path);
    return mode == PermissionMode.whitelist ? isInList : !isInList;
  }
}
```

**Go 端** - [go_lib/core/sandbox/seatbelt.go](go_lib/core/sandbox/seatbelt.go:22-25)
```go
type PathPermissionConfig struct {
    Mode  PermissionMode `json:"mode"`
    Paths []string       `json:"paths"`
}
```

### 5.3 NetworkPermissionConfig（网络权限）

支持入栈（Inbound）和出栈（Outbound）分离配置：

```dart
class NetworkPermissionConfig {
  final DirectionalNetworkConfig outbound;  // 出栈规则
  final DirectionalNetworkConfig inbound;   // 入栈规则
}

class DirectionalNetworkConfig {
  final PermissionMode mode;
  final List<String> addresses;  // 支持 *, localhost, 127.0.0.1
}
```

**Go 端** - [go_lib/core/sandbox/seatbelt.go](go_lib/core/sandbox/seatbelt.go:33-39)
```go
type NetworkPermissionConfig struct {
    Inbound  DirectionalNetworkConfig `json:"inbound"`
    Outbound DirectionalNetworkConfig `json:"outbound"`
}
```

### 5.4 ShellPermissionConfig（Shell 权限）

```dart
class ShellPermissionConfig {
  final PermissionMode mode;
  final List<String> commands;  // 命令或命令前缀
}
```

### 5.5 沙箱策略生成

权限设置最终转换为平台对应的沙箱策略:

**macOS**: Seatbelt 策略文件 (sandbox-exec)
```go
// go_lib/core/sandbox/seatbelt.go
func (p *SeatbeltPolicy) GeneratePolicy() (string, error)
```
策略文件路径: `~/.botsec/policies/botsec_{asset_name}.sb`

**Linux**: LD_PRELOAD 策略 JSON (libsandbox_preload.so)
```go
// go_lib/core/sandbox/preload_linux.go
func buildPreloadConfig(config SandboxConfig) *PreloadConfig
```
策略文件路径: `~/.botsec/policies/botsec_{asset_name}_preload.json`

Linux LD_PRELOAD 策略通过以下方式注入:
- systemd unit 添加 `Environment=LD_PRELOAD=...` 和 `Environment=SANDBOX_POLICY_FILE=...`
- preload 库在进程启动时加载策略，拦截 open/openat/connect/getaddrinfo/system/execve
- 支持黑名单/白名单模式，网络拦截同时支持 IP 和域名(域名自动解析为 IP 提供双重覆盖)

---

## 6. AppSettings（应用设置）

### 6.1 数据结构

采用 Key-Value 存储：

```sql
CREATE TABLE app_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

### 6.2 预定义设置键

```go
// go_lib/core/repository/app_settings_repository.go
const (
    SettingKeyLanguage      = "language"        // 语言设置
    SettingKeyIsFirstLaunch = "is_first_launch" // 首次启动标记
)
```

### 6.3 服务层接口

**Go 端** - [go_lib/core/service/app_settings_service.go](go_lib/core/service/app_settings_service.go)
```go
func SaveAppSetting(jsonStr string) map[string]interface{}
func GetAppSetting(key string) map[string]interface{}
func SetLanguage(lang string) map[string]interface{}
func GetLanguage() map[string]interface{}
```

**Flutter 端** - [lib/services/app_settings_database_service.dart](lib/services/app_settings_database_service.dart)
```dart
class AppSettingsDatabaseService {
  Future<bool> saveSetting(String key, String value);
  Future<String> getSetting(String key);
  Future<bool> isFirstLaunch();
  Future<bool> setFirstLaunchCompleted();
}
```

---

## 7. ShepherdGate UserRules（用户规则）

### 7.1 规则结构

```go
// go_lib/plugins/openclaw/shepherd_gate.go
type UserRules struct {
    SensitiveActions []string  // 需要用户确认的敏感操作
}
```

### 7.2 规则更新机制

用户规则存储在 Go 层全局变量中，通过 FFI 更新：

```go
var globalUserRules *UserRules
var globalRulesMu sync.RWMutex

func UpdateGlobalUserRules(sensitiveActions []string) {
    globalRulesMu.Lock()
    defer globalRulesMu.Unlock()
    globalUserRules = &UserRules{
        SensitiveActions: sensitiveActions,
    }
}
```

### 7.3 Flutter 端调用

```dart
// lib/services/plugin_service.dart
Future<void> updateShepherdRules(List<String> sensitiveActions) async {
    final rulesMap = {'SensitiveActions': sensitiveActions};
    final jsonStr = jsonEncode(rulesMap);
    // 调用 UpdateShepherdRulesFFI
}
```

---

## 8. 配置启动时的加载流程

### 8.1 ProtectionConfig 聚合结构

代理启动时，Flutter 构建聚合配置传递给 Go 层：

**Go 端** - [go_lib/plugins/openclaw/model_config.go](go_lib/plugins/openclaw/model_config.go:92-112)
```go
type ProtectionConfig struct {
    SecurityModel *SecurityModelConfig       // 安全模型
    BotModel      *BotModelConfig            // Bot 模型
    Runtime       *ProtectionRuntimeConfig   // 运行时配置
    
    // 基线统计（从数据库恢复）
    BaselineAnalysisCount         int
    BaselineBlockedCount          int
    BaselineWarningCount          int
    BaselineTotalTokens           int
    BaselineTotalPromptTokens     int
    BaselineTotalCompletionTokens int
    BaselineTotalToolCalls        int
    BaselineRequestCount          int
    BaselineAuditTokens           int
    BaselineAuditPromptTokens     int
    BaselineAuditCompletionTokens int
}
```

### 8.2 启动时序

```
┌─────────────────────────────────────────────────────────────────────────┐
│ 1. Flutter: ProtectionService.startProtectionProxy()                    │
│    └─> 加载 SecurityModelConfig                                         │
│    └─> 加载 BotModelConfig                                              │
│    └─> 加载 ProtectionRuntimeConfig                                     │
│    └─> 加载基线统计数据                                                  │
│                                                                         │
│ 2. 构建 configPayload JSON                                              │
│    └─> FFI: StartProtectionProxyFFI                                     │
│                                                                         │
│ 3. Go: NewProxyProtectionFromConfig()                                   │
│    └─> 解析 SecurityModel -> 创建 ShepherdGate                          │
│    └─> 解析 BotModel -> 创建 ForwardingProvider                         │
│    └─> 解析 Runtime -> 设置 Token 限制                                   │
│    └─> 加载基线统计到内存                                                │
│    └─> 更新 openclaw.json                                               │
│    └─> 启动 HTTP 代理服务器                                              │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 9. 配置热更新支持

| 配置类型 | 热更新支持 | 更新方法 |
|---------|----------|---------|
| SecurityModelConfig | 支持 | `UpdateSecurityModelConfig()` |
| BotModelConfig | 支持 | `updateBotForwardingProvider()` |
| ProtectionRuntimeConfig | 支持 | `UpdateProtectionConfig()` |
| Token 限制 | 支持 | `pushTokenLimitsToProxy()` |
| ShepherdGate 规则 | 支持 | `UpdateGlobalUserRules()` |
| 沙箱权限 | 支持 | `syncGatewaySandbox()` 热同步策略并重启受沙箱保护的 Bot 进程 |

---

## 10. 配置界面组件

| 组件 | 配置对象 | 位置 |
|------|---------|------|
| SecurityModelConfigForm | SecurityModelConfig | lib/widgets/security_model_config_form.dart |
| BotModelConfigForm | BotModelConfig | lib/widgets/bot_model_config_form.dart |
| ProtectionConfigDialog | ProtectionConfig | lib/widgets/protection_config_dialog.dart |

---

## 11. 关键设计原则

### 11.1 职责分离

- **SecurityModelConfig**: 仅用于 ShepherdGate 风险检测
- **BotModelConfig**: 仅用于代理转发目标
- 两者完全独立，不互相回退或影响

### 11.2 分层架构

```
Flutter UI
    ↓
Service Layer (*ConfigService)
    ↓
Database Service (*DatabaseService)
    ↓
FFI Layer
    ↓
Go Service Layer (core/service)
    ↓
Go Repository Layer (core/repository)
    ↓
SQLite
```

### 11.3 数据协议

- 所有 FFI 通信使用 JSON
- 响应格式: `{"success": bool, "data": ..., "error": ...}`
- Go 端使用 `json.Marshal`，禁止 `fmt.Sprintf` 拼接

---

## 12. 相关文件索引

### Flutter 端
- `lib/models/llm_config_model.dart` - 模型配置数据结构
- `lib/models/protection_config_model.dart` - 防护配置数据结构
- `lib/services/model_config_service.dart` - 模型配置服务
- `lib/services/model_config_database_service.dart` - 模型配置数据库服务
- `lib/services/protection_service.dart` - 防护服务
- `lib/services/protection_database_service.dart` - 防护配置数据库服务
- `lib/services/app_settings_database_service.dart` - 应用设置数据库服务

### Go 端
- `go_lib/plugins/openclaw/model_config.go` - 模型配置结构定义
- `go_lib/plugins/openclaw/proxy_protection.go` - 代理防护实现
- `go_lib/plugins/openclaw/shepherd_gate.go` - ShepherdGate 实现
- `go_lib/core/repository/security_model_config_repository.go` - 安全模型配置仓库
- `go_lib/core/repository/protection_repository.go` - 防护配置仓库
- `go_lib/core/repository/app_settings_repository.go` - 应用设置仓库
- `go_lib/core/service/security_model_config_service.go` - 安全模型配置服务
- `go_lib/core/service/bot_model_config_service.go` - Bot 模型配置服务
- `go_lib/core/service/app_settings_service.go` - 应用设置服务
- `go_lib/core/sandbox/seatbelt.go` - macOS Seatbelt 沙箱策略生成
- `go_lib/core/sandbox/macos_hook/` - macOS 沙箱入口目录(当前由 Seatbelt 方案实现)
- `go_lib/core/sandbox/preload_linux.go` - Linux LD_PRELOAD 沙箱策略生成
- `go_lib/core/sandbox/manager_linux.go` - Linux 沙箱管理器
- `go_lib/plugins/openclaw/gateway_platform_linux.go` - Linux systemd unit 注入与网关管理
- `go_lib/core/sandbox/linux_hook/preload.c` - Linux LD_PRELOAD 沙箱源码(拦截 open/openat/connect/getaddrinfo/system/execve 等)
