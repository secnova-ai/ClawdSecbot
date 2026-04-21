# 审计链路规范（AuditChain）

> 适用范围：代理防护链路中的审计日志重构与维护。  
> 本文档描述审计功能实现细则；`_rules/global.md` 仅保留高层约束与索引。

## 1. 目标与职责

- `TruthRecord`：请求级实时快照，服务监控卡片与调试流。
- `AuditChain`：任务级审计链路，服务审计日志持久化与回放。
- 两者允许并存，但同一视图只允许一个主数据源。

## 2. 链路定义（强约束）

- 开始条件：当拦截到 `messages` 且最后一条为 `role=user`，创建一条新审计日志并生成 `log_uuid`。
- 关联规则：
  - LLM 返回 `tool_call` 时，用 `tool_call_id -> log_uuid` 建立内存关联；
  - agent 回传 `tool_result` 时，用 `tool_call_id` 命中关联并写入对应链路；
  - 允许 ID 归一化（大小写/分隔符差异）；
  - 若 provider 未返回 `tool_call_id`，代理可生成短 ID，并保证请求与响应链路一致。
- 结束条件：当 LLM 返回非 `tool_call` 的 assistant 内容时，更新该链路 `final_output`。
- 不要求 `status` 字段；用户中断导致“无 `tool_result` / 无 `final_output`”是合法状态。
- 审计日志不得记录系统提示词（system prompt）作为 `request_content`。
- 并发约束：请求归属必须依赖请求上下文绑定，禁止依赖全局可变“当前 request_id”。
- 审计异常不得阻断代理主流程（fail-open for audit）。

## 3. 数据与持久化口径

- 主表至少包含：`log_uuid`、`request_content`、`final_output`、`asset_id`、`timestamp`。
- 明细表：`tool_call` / `tool_result`，通过外键（`log_uuid`）或等价关联键连接。
- 审计写入必须支持幂等更新，避免乱序写覆盖新值。
- 关联缓存必须具备 TTL 清理：
  - `request -> log`
  - `tool_call -> log`
  - pending tool result / pending request link / pending final output

## 4. 流式输出要求

- 流式场景下，`tool_call_id` 在 chunk 间必须稳定。
- 若代理改写响应中的 `tool_call_id`，必须同步改写透传给下游的 raw payload，保证上游观测与下游执行一致。

## 5. 回归检查清单

- 并发请求不串链：A 请求的 `tool_result` / `assistant` 不得落到 B 的 `log_uuid`。
- 多轮消息不合并：不得出现 `user->...->assistant->user->...` 被写入单条审计链。
- 允许中断残缺：没有 tool 明细或没有 final 输出时，日志仍可保留。
- 审计失败不影响代理转发：持久化错误只记日志，不中断请求。
