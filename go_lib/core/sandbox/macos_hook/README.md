# macOS Sandbox Hook Entry

当前 macOS 平台不使用独立的 native hook 动态库。

- 沙箱策略生成：`go_lib/core/sandbox/seatbelt.go`
- 启动注入逻辑：`go_lib/core/sandbox/manager_darwin.go`
- 网关编排逻辑：`go_lib/plugins/openclaw/gateway_platform_darwin.go`

保留 `macos_hook` 目录用于与 `linux_hook`、`windows_hook` 统一目录语义，后续若需要 native 扩展可直接落在该目录。
