# Hermes 插件

**[English](README.md)**

ClawSecbot 的 Hermes 防护插件。

该插件用于将 [NousResearch/hermes-agent](https://github.com/NousResearch/hermes-agent) 接入仓库统一防护链路，覆盖资产发现、风险评估、风险缓解、网关生命周期和代理防护。

## 作用范围

本目录实现 `Hermes` 资产类型的插件能力：

- 插件注册与清单定义
- Hermes 资产扫描与元数据补充
- 配置驱动的风险检测
- 风险缓解分发与配置修复
- 网关重启与配置备份/恢复
- 生命周期钩子（`OnProtectionStart`、`OnBeforeProxyStop`）
- 模型连通性测试能力

## 架构位置

插件遵循项目标准调用链：

`Flutter -> FFI(go_lib/main.go) -> core -> plugins/hermes`

`go_lib/main.go` 已接入本插件运行时参数路由：

- `SetConfigPathFFI` -> `hermes.SetConfigPath`
- `SetAppStoreBuildFFI` -> `hermes.SetAppStoreBuild`

## 配置发现规则

Hermes 配置文件按以下顺序查找：

1. 通过 `SetConfigPath(...)` 显式覆盖
2. 环境变量 `HERMES_CONFIG`
3. 环境变量 `HERMES_HOME/config.yaml`
4. 默认路径：
   - `~/.hermes/config.yaml`
   - `~/.hermes/config.yml`
   - `~/.config/hermes/config.yaml`
   - `~/.config/hermes/config.yml`

## 使用的配置字段

当前插件会读取或写入这些字段：

- `model.default`
- `model.provider`
- `model.base_url`
- `model.api_key`
- `terminal.backend`
- `approvals.mode`
- `security.redact_secrets`

## 资产标识

- 资产类型：`Hermes`
- `asset_id` 由 `core.ComputeAssetID(...)` 生成
- 为避免运行态端口/进程波动导致实例漂移，资产指纹固定采用 `config_path`

## 风险检测

当前风险 ID：

- `config_perm_unsafe`
- `config_dir_perm_unsafe`
- `terminal_backend_local`
- `approvals_mode_disabled`（`off` / `never` / `yolo`）
- `redact_secrets_disabled`
- `model_base_url_public`

缓解模板定义见 [`mitigation.json`](./mitigation.json)。

## 自动缓解

当前支持的 `form` 自动缓解：

- 修正文件权限为 `0600`
- 修正目录权限为 `0700`
- 设置 `security.redact_secrets=true`
- 设置 `approvals.mode` 为 `manual` 或 `smart`

## 防护生命周期

### 启动时（`OnProtectionStart`）

1. 按 `asset_id` 读取防护配置
2. 读取 Bot 转发模型配置
3. 备份 Hermes 配置到：
   - `<backup_dir>/hermes/<sanitized_asset_id>/config.yaml.bak`
4. 将 Hermes 模型配置改写为本地代理：
   - `model.provider = custom`
   - `model.base_url = http://127.0.0.1:<proxy_port>`
   - `model.api_key = botsec-proxy-key`
   - `model.default = <bot_model.model>`
5. 重启 Hermes gateway

### 停止前（`OnBeforeProxyStop`）

1. 按 `asset_id` 恢复备份配置
2. 重启 Hermes gateway

## Gateway 重启策略

优先执行：

- `hermes gateway restart`

若失败则降级：

- `hermes gateway stop`
- `hermes gateway start`

`hermes` 可执行文件通过 `PATH` 查找。

## App Store 限制

当启用 `SetAppStoreBuild(true)` 时，插件会阻断配置改写和网关重启路径，并返回失败结果（设计使然）。

## 测试

在 `go_lib` 目录执行：

```bash
go test ./plugins/hermes
go test ./...
```

当前插件目录包含 scanner/checker/config/mitigation/gateway/proxy session/plugin lifecycle 的单元测试。
