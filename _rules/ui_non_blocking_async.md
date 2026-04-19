# ClawSecbot UI 非阻塞异步开发规范

> 目标：任何耗时操作都不能阻塞主 UI 线程；用户必须先看到界面，再看到明确的 loading 与进度反馈。

## 1. 适用范围

- Flutter 侧所有可能超过 `16ms` 的操作：
  - 初始化链路（DB/插件/FFI/状态恢复）
  - 防护启停、配置恢复、批量清理
  - 大 JSON 解析、批量数据转换
  - 任何 Go FFI 调用（尤其启动/停止/扫描/恢复类）

## 2. 基本原则（必须遵守）

- UI 优先：先渲染可见界面，再启动重任务。
- 状态优先：先设置 loading 状态，再执行 `await`。
- 主线程最小化：主 isolate 只做轻量状态更新与渲染。
- 长任务异步化：耗时 FFI/CPU 操作必须下沉到后台 isolate 或 Go 侧异步执行。
- 平台隔离：IO 与 Web 的执行路径必须分离（conditional export），禁止把 `dart:ffi` 直接引入跨平台公共服务。
- 禁止“假非阻塞”：`async` 包装但内部仍在主线程做重计算，视为违规。

## 3. 启动流程规范

- 禁止在 `initState` 中直接 `await` 重初始化任务。
- 使用 `addPostFrameCallback` 先完成首帧，再触发启动链路。
- 欢迎页/首屏结束后，先 `await Future<void>.delayed(Duration.zero)` 让出事件循环，再启动重任务。
- 扫描结果、保护态等可分阶段恢复：
  - 先恢复 UI 可展示的数据（例如上次扫描结果）
  - 再后台恢复防护代理
- 防护恢复期间，按钮必须显示 loading，禁止“无反馈等待”。

## 4. 退出与恢复流程规范

- 退出动作必须区分明确路径：
  - `恢复并退出`：执行恢复链路 + 进度提示
  - `不恢复直接退出`：跳过恢复链路，快速退出
- 批量退出清理（逐资产 stop/restore）必须异步逐项执行，并在循环内主动让出事件循环（如 `Future<void>.delayed(Duration.zero)`）。
- 退出过程中必须防重入（例如 `_isExitFlowInProgress`）。
- 有耗时清理时必须显示阻塞层或进度对话框，避免用户误判为卡死。

## 5. FFI 与 Isolate 规范

- 任何可能阻塞的 FFI 调用必须放在后台 isolate 执行。
- `Isolate.run` 闭包禁止捕获 `this` 或不可发送对象；仅传入纯值参数（`String/int/bool/Map/List` 可发送对象）。
- 建议通过执行器封装隔离调用（如 `ProtectionProxyExecutor`）：
  - IO：`Isolate.run + FFI`
  - Web：`Transport RPC`
- 大报文 JSON（如日志/TruthRecord）解析需要阈值策略，超过阈值走后台 isolate。

## 6. 平台抽象规范（防回归）

- 业务服务层不直接依赖 `native_library_service.dart`、`dart:ffi` 等 IO 专属实现。
- 使用 `xxx.dart` + `xxx_io.dart` + `xxx_web.dart` 条件导出模式封装平台差异。
- 每次改动跨层调用后，必须验证 Web 构建不因 FFI 链路回归而失败。

## 7. UI 交互规范

- 任何耗时按钮都需要：
  - loading 状态
  - 禁止重复点击
  - 失败后可见错误反馈
- loading 状态必须在 `finally` 中兜底清理，防止异常后 UI 永久锁死。
- 异步任务完成后更新 UI 前必须检查 `mounted`。

## 8. 禁止事项

- 禁止在主线程同步执行重 FFI 调用。
- 禁止在主线程同步解析超大 JSON。
- 禁止在欢迎页/过渡页显示期间串行执行全部重初始化，导致主界面长时间不可见。
- 禁止为了“兼容历史”堆叠无必要降级逻辑；优先做根因修复。

## 9. 验收清单（PR 必检）

- [ ] 启动后主界面是否能快速显示（先 UI，后恢复）  
- [ ] 防护启停/恢复时按钮是否有 loading 且可感知  
- [ ] 退出两条路径（恢复/不恢复）是否都无卡死  
- [ ] 是否存在主线程重 FFI 或重 JSON 解析  
- [ ] `Isolate.run` 是否仅使用可发送参数，无 `this` 捕获  
- [ ] Web 构建是否通过（至少 `flutter build web --target lib/main_web.dart --debug`）  
- [ ] `flutter analyze` 是否通过  

