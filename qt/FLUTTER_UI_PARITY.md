# Flutter -> Qt 桌面界面对照与验收清单

基准应用由仓库根目录 `scripts/run_with_pprof.sh` 启动。Qt 应用由
`qt/scripts/run_with_pprof.sh` 启动。此表只记录已经从 Flutter 实机画面和
可访问性树核对过的状态，不以“代码中存在同名窗口”替代验收。

## 核心窗口

| Flutter 界面/状态 | Qt 实现 | 视觉 | 交互/业务 | 当前结论 |
|---|---|---:|---:|---|
| 主窗口标题栏、扫描完成头部 | `MainWindow` | 已实机对照 | 设置、审计、重扫可点击 | 第一轮通过，图标字形仍需统一 |
| 折叠资产卡 | `AssetCardWidget` | 已实机对照 | 头部可展开、图标可选 | 通过 |
| 展开资产详情 | `AssetCardWidget` | 已核对真实 Openclaw 数据 | 一键防护/监控/停止/配置按状态切换 | 通过 |
| 风险卡 | `MainWindow::renderResult` | 已实机对照图标色块、等级、Bot 徽章、修复按钮 | 普通修复进入表单，`skills_not_scanned` 转 Skill 扫描 | 通过 |
| 全局设置：安全模型 | `SettingsDialog` | 已实机对照 | 加载、编辑、验证、保存均走 Go FFI | 通过 |
| 全局设置：通用设置 | `SettingsDialog` | 已实机对照 | 自启写入平台注册项；定时扫描配置驱动 Qt 定时器；API 服务按持久化设置恢复；清理、恢复、关于均接入 | 运行态与持久化回归通过，平台启动项仍需 Windows/Linux CI 验证 |
| 防护配置：智能规则 | `ProtectionConfigDialog` | 已按 Flutter 重建卡片与滚动结构 | 审计/输入开关、自定义规则、内置规则开关可操作 | 第一轮通过 |
| 防护配置：Token 限制 | `ProtectionConfigDialog` | 已实机对照预设与提示卡 | 预设与数值输入双向联动 | 第一轮通过 |
| 防护配置：权限设置 | `ProtectionConfigDialog` | 已按沙箱、路径、网络、Shell 卡片重建并截图复核 | 开关、黑白名单、输入均写入原 Go JSON | 通过 |
| 防护配置：Bot 模型 | `ProtectionConfigDialog` | 已实机对照，去除 Flutter 中不存在的 Secret Key 可见项 | Provider、URL、Key、Model、连通性验证可操作 | 通过 |
| 审计日志 | `AuditLogWindow` + `AuditTimelineWidget` | 卡片列表和右侧可滚动详情面板；已按 Flutter 语义合并 messages 与 tool call/result 时间线 | 筛选、搜索、仅风险、选择、导出、刷新、清空、详情、时间线回放 | 结构与模型回归通过，实机样本已复核 |
| 防护监控 | `ProtectionMonitorWindow` | 8 指标、双趋势图、双日志视图、决策与事件列表已实现 | 状态/日志/事件轮询、审计模式、事件详情均连接 Go；离屏 UI 冒烟测试通过 | 结构和交互通过，真实流量动态值待防护会话复核 |

## 其他界面

| Flutter 界面 | Qt 实现 | 当前状态 |
|---|---|---|
| 首次引导 | `OnboardingDialog` | 自动首启、四步切换和 Bot 模型按真实 `asset_id` 读写已实机点击；右上角可关闭并从主菜单重新打开；完成动作待非测试环境确认 |
| 配置引导 | `AppStoreGuideDialog` | 已与当前四步配置流程统一，完成按钮可用 |
| Skill 扫描 | `SkillScanDialog` | 已实现自动启动、进度/日志轮询、结果卡、信任与删除；已实机完成“无待扫描 Skill”完整闭环 | 空态通过，有任务的模型扫描动态值待后续样本复核 |
| Skill 历史 | `SkillScanResultsDialog` | 已实机截图为 Flutter 状态卡，包含时间、路径、风险展开和空态 | 通过 |
| 风险修复 | `MitigationDialog` | 已实机打开；表单类型、建议分组、复制命令和 `MitigateRiskFFI` 均接入 | 建议型通过，自动修复执行未在测试中触发 |
| Bot 图标选择 | `BotIconPickerDialog` | 已实机验证 28 图标、8 色、实时预览和选择状态 | 通过 Go AppSetting 与 Flutter 共用持久化 |

## 验收门槛

- Qt 源文件保持少于 1500 行。
- 所有 Go 调用在 `QtConcurrent` 后台任务中执行，不阻塞 UI 线程。
- 防护接口只传 `asset_id`，资产名称仅用于展示或 Go 明确要求的兼容参数。
- 每个页面都要完成：可见性、点击/输入、加载态、失败态、关闭/返回五类检查。
- 只有实机点击和构建测试均通过后，才把对应项标为最终通过。
