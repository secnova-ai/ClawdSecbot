# Openclaw 类 Bot 插件适配指南

**[English](openclaw_like_bot_plugin_guide.md)**

本文说明如何通过插件机制，把 ClawSecbot 适配到其它 Openclaw 类 Bot。

## 1. 适用范围

当目标 Bot 与 Openclaw 架构相近时，使用本指南：

- 具有本地工作区、配置文件、进程、网关。
- 通过 HTTP/HTTPS 与 LLM API 交互。
- 可以通过改配置并重启进程实现流量改道。

## 2. 核心原则

1. 资产 ID 是唯一实例标识。
2. 资产实例与插件实例严格 1:1。
3. 插件类型注册与运行时实例绑定分离。
4. core 层保持 Bot 无关，不写 bot 专属逻辑。

## 3. 运行时模型

- 类型层：插件通过 `asset_name` 注册到 `PluginManager`。
- 实例层：每个扫描到的 `asset_id` 绑定到唯一插件实例条目。
- 运行时操作统一以 `(assetName, assetID)` 进入，最终按 `assetID` 路由。

代码参考：

- `go_lib/core/plugin.go`
- `go_lib/core/plugin_manager.go`
- `go_lib/core/asset.go`

## 4. 必需接口

在插件包中实现 `core.BotPlugin`：

```go
type BotPlugin interface {
    GetAssetName() string
    ScanAssets() ([]Asset, error)
    AssessRisks(scannedHashes map[string]bool) ([]Risk, error)
    MitigateRisk(riskInfo string) string
    StartProtection(assetID string, config ProtectionConfig) error
    StopProtection(assetID string) error
    GetProtectionStatus(assetID string) ProtectionStatus
}
```

可选生命周期钩子：

```go
type ProtectionLifecycleHooks interface {
    OnProtectionStart(ctx *ProtectionContext) (map[string]interface{}, error)
    OnBeforeProxyStop(ctx *ProtectionContext)
}
```

`OnProtectionStart` 是 Bot 专属流量劫持的主要实现点（例如：改 bot 配置、指向本地代理、重启网关进程）。

## 5. 资产发现与确定性 ID

### 5.1 构建稳定指纹

使用 `core.ComputeAssetID(name, configPath)` 生成确定性 ID。

规则：

- 指纹输入仅限 `name` + `config_path`，两者都要稳定、可规范化。
- **禁止**把 `ports`、`process_paths`、`pid`、`service_name` 等运行态动态信息卷入指纹，否则 bot 启停会让 `asset_id` 漂移，出现同一资产两条记录、启用防护后策略丢失等问题。
- 不要引入易变字段（时间戳、临时路径、随机值）。

### 5.2 SourcePlugin

每个发现的资产都要设置 `asset.SourcePlugin = plugin.GetAssetName()`。

### 5.3 共享运行时认领

两个产品可以保持不同的用户展示身份，同时识别同一个兼容运行时。相同的
runtime family 与 canonical `config_path` 用于识别该共享运行时；端口、PID、
进程路径等只属于产品证据，不得影响 `asset_id`。

当插件与其它插件共享运行时 family 时，实现可选的
`core.RuntimeAssetClaimProvider`。claim 必须返回 canonical config path、
variant ID 和确定性优先级；只有通用兼容插件可以设置 `IsFallback`。不得用
插件注册顺序决定归属。

`ScanAllAssets` 是权威仲裁路径：它选出唯一赢家，只绑定该插件实例，并将
风险与防护路由交给获胜产品。即使为兼容性复用通用 fallback 的 asset ID，
产品插件仍可保留自己的展示名称和 UI 区块。

普通 claim 仲裁不得静默迁移已持久化的跨产品 asset ID。此类迁移必须显式
执行数据库迁移并同步应用版本。

## 6. 多实例防护流程

1. 执行资产扫描（`ScanAllAssets`），并按资产 ID 绑定插件实例。
2. 用户选择具体资产，点击一键防护。
3. UI 传入 `assetName + assetID` 到 FFI。
4. core 路由到该资产实例对应插件，启动代理/沙箱。
5. 状态查询、停止防护都使用同一 `assetID`。

重点：

- 不存在独立的插件实例 ID。
- 资产 ID 就是插件实例标识。
- 一个共享运行时在聚合扫描中只能有一个赢家和一个路由归属。

## 7. 建议的适配步骤

1. 在 `go_lib/plugins/<yourbot>/` 创建插件包。
2. 新建插件结构体，维护每资产状态表（`map[assetID]ProtectionStatus`）。
3. 在 `init()` 中 `core.GetPluginManager().Register(plugin)`。
4. 实现 scanner。
5. 实现风险评估与一键修复模板。
6. 实现防护钩子。
7. 验证同类型多个资产可并发独立防护。

## 8. 模型配置时机建议

Openclaw 类产品首次扫描前通常没有资产 ID，模型配置不应强绑定资产。

建议流程：

1. 首次启动不强制配置 bot 模型。
2. 扫描资产后，按资产打开防护配置弹窗。
3. 保存时先校验 bot 模型。
4. 若安全模型缺失，再弹安全模型配置。
5. 支持“复用 bot 模型配置”。

## 9. Core 与插件职责边界

Core 职责：

- 插件注册与路由。
- ShepherdGate 与通用安全技能。
- LLM 代理转发、审计、回调桥接。
- DB/repository/service 等公共能力。

插件职责：

- Bot 专属资产识别。
- Bot 专属风险规则与一键修复。
- Bot 专属流量劫持机制。
- Bot 专属进程重启与恢复细节。

## 10. 验证清单

- 资产 ID 多次扫描保持稳定。
- 不同资产生成不同 ID。
- 一个资产 ID 只对应一个运行时插件实例条目。
- 启停/状态查询都命中正确 `assetID`。
- `OnProtectionStart` 只修改目标资产实例。
- `OnBeforeProxyStop` 回滚逻辑具备幂等性。
- 多个资产窗口可独立运行防护。
- 日志、统计、风险事件可实时回传 UI。
- `./scripts/run_with_pprof.sh` 可编译并启动。

## 11. 最小代码骨架

```go
package yourbot

import (
    "sync"
    "go_lib/core"
)

type YourBotPlugin struct {
    mu sync.RWMutex
    protectionStatuses map[string]core.ProtectionStatus
}

var plugin *YourBotPlugin

func init() {
    plugin = &YourBotPlugin{protectionStatuses: make(map[string]core.ProtectionStatus)}
    core.GetPluginManager().Register(plugin)
}

func (p *YourBotPlugin) GetAssetName() string { return "yourbot" }

func (p *YourBotPlugin) ScanAssets() ([]core.Asset, error) {
    // 扫描 config/process/ports；仅用名称和 canonical 配置路径计算确定性 assetID
    return nil, nil
}

func (p *YourBotPlugin) AssessRisks(scannedHashes map[string]bool) ([]core.Risk, error) {
    return nil, nil
}

func (p *YourBotPlugin) MitigateRisk(riskInfo string) string { return `{"success":true}` }

func (p *YourBotPlugin) StartProtection(assetID string, cfg core.ProtectionConfig) error {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.protectionStatuses[assetID] = core.ProtectionStatus{Running: true, ProxyRunning: true, ProxyPort: cfg.ProxyPort}
    return nil
}

func (p *YourBotPlugin) StopProtection(assetID string) error {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.protectionStatuses[assetID] = core.ProtectionStatus{Running: false, ProxyRunning: false}
    return nil
}

func (p *YourBotPlugin) GetProtectionStatus(assetID string) core.ProtectionStatus {
    p.mu.RLock()
    defer p.mu.RUnlock()
    return p.protectionStatuses[assetID]
}
```

## 12. 备注

- 保持项目约定：中文注释、英文日志。
- 为 scanner、ID 稳定性、多实例防护行为补充单测。
- 避免在 `go_lib/core` 添加 bot 专属分支逻辑。
