# ShepherdGate 并行用户指令隔离技术方案

## 1. 背景

当前 ShepherdGate 已经实现了从 `UserInput -> ToolCall -> ToolCallResult -> RequestRewrite -> FinalResult` 的全链路安全检测，但运行时状态仍有一部分挂在 `ProxyProtection` 实例上。

这些状态包括：

- 当前流式响应缓冲：`streamBuffer`
- 最近请求上下文：`lastContextMessages`
- 最近用户输入：`lastUserMessageContent`
- 待确认恢复状态：`pendingRecovery`
- 被隔离的 tool call：`blockedToolCallIDs`

在单用户、单会话、串行请求下，这些状态可以正常工作。但如果被防护的 claw 是多智能体架构，或者同一个代理实例同时处理多个用户指令，就会出现状态覆盖和乱序问题。

典型风险：

- A 请求的流式响应尚未结束，B 请求进入后清空了全局 `streamBuffer`，导致 A 的后续 chunk 被写进 B 的 buffer。
- A 的 tool result 到来时，`lastContextMessages` 已经被 B 请求覆盖，导致 ShepherdGate 拿错上下文做安全判断。
- A 和 B 都触发 ShepherdGate 确认时，全局 `pendingRecovery` 只能保存一个，后一个会覆盖前一个。
- 被阻断的 `tool_call_id` 只按 ID 存储，没有按用户指令链隔离，存在交叉确认、交叉改写风险。

因此，需要引入“用户指令链级别”的运行时隔离机制。

## 2. 约束

ClawdSecbot 是通用代理，不能要求被防护的 claw 一定提供：

- `agent_id`
- `session_id`
- `conversation_id`
- `user_instruction_id`

也不能依赖某个特定模型供应商或某个特定 Agent 框架的私有字段。

因此方案必须满足：

- 只依赖 OpenAI-compatible 消息协议中已有的信息。
- 代理可以自己生成隔离 ID。
- 上游不提供业务身份时仍能尽可能隔离。
- 对无法可靠关联的场景允许降级，但不能让安全检测完全失效。

## 3. 目标

### 3.1 必须达成

- 并行用户指令之间的运行时状态互相隔离。
- `tool_call`、`tool_result`、`final_result` 能关联到同一条用户指令链。
- 被阻断的 `tool_call_id` 隔离状态不再是全局单槽，而是绑定到指令链。
- 用户确认/取消只影响对应指令链，不影响其它并行请求。
- 流式响应缓冲按请求隔离，不能互相覆盖。
- RequestRewrite 只改写对应指令链中被阻断的 tool result。
- 安全事件和审计日志能体现 `instruction_chain_id`。

### 3.2 允许降级

以下情况允许降级，但必须记录安全流程日志：

- tool result 缺少 `tool_call_id`，无法可靠回链。
- 上游复用相同 `tool_call_id`。
- 软件重启后内存链路丢失，只能从历史 `messages` 尽力恢复。
- 请求历史被上游压缩，缺少完整 tool call/tool result 配对。
- 多条并行链路共享完全相同的历史消息且没有可区分 tool id。

降级原则：

- 不确定归属时，不静默放行高风险内容。
- 对高风险 tool result 优先确认或隔离。
- 对低风险且无法关联的内容允许审计后放行。
- 所有降级路径必须打 `[ShepherdGate][Flow]` 日志。

## 4. 核心设计

引入代理自生成的 `instruction_chain_id`。

它不是上游传入的业务 session，也不是审计日志的持久化 ID，而是 ShepherdGate 运行时使用的安全隔离 ID。

```text
instruction_chain_id = "sg_chain_" + request_id 或随机/单调生成 ID
```

核心关联关系：

```text
request_id -> instruction_chain_id
tool_call_id -> instruction_chain_id
instruction_chain_id -> SecurityChainState
```

其中 `SecurityChainState` 保存该用户指令链的安全上下文。

## 5. 数据结构设计

### 5.1 SecurityChainState

建议新增：

```go
type SecurityChainState struct {
    ChainID string
    AssetID string

    CreatedAt time.Time
    UpdatedAt time.Time
    ExpiresAt time.Time

    RootRequestID string
    RequestIDs map[string]struct{}
    ToolCallIDs map[string]struct{}

    LastContextMessages []ConversationMessage
    LastUserMessageContent string

    PendingRecovery *pendingToolCallRecovery
    PendingRecoveryArmed bool

    BlockedToolCallIDs map[string]time.Time
}
```

### 5.2 RequestRuntimeState

建议新增：

```go
type RequestRuntimeState struct {
    RequestID string
    ChainID string
    CreatedAt time.Time
    ExpiresAt time.Time

    StreamBuffer *StreamBuffer
    RawBody []byte
    Messages []openai.ChatCompletionMessageParamUnion
}
```

### 5.3 ProxyProtection 新增索引

建议在 `ProxyProtection` 中新增：

```go
chainMu sync.Mutex
chains map[string]*SecurityChainState
requestToChain map[string]string
toolCallToChain map[string]string
requestStates map[string]*RequestRuntimeState
```

TTL 建议：

| 状态 | TTL |
| --- | --- |
| `requestStates` | 30 分钟 |
| `requestToChain` | 2 小时 |
| `toolCallToChain` | 2 小时 |
| `chains` | 2 小时，或最后更新时间后 2 小时 |
| blocked `tool_call_id` | 2 小时 |

## 6. 链路创建与关联规则

### 6.1 用户输入请求

当请求最后一条消息是 `role=user`，并且不是 tool result 请求时：

1. 创建新的 `instruction_chain_id`。
2. 绑定 `request_id -> instruction_chain_id`。
3. 创建 `SecurityChainState`。
4. 保存当前 `messages` 和最后一条用户输入。
5. 创建 `RequestRuntimeState`，其中包含独立 `StreamBuffer`。

```text
user request req_1
  -> create chain sg_chain_1
  -> request_id(req_1) -> sg_chain_1
```

### 6.2 assistant 返回 tool_call

当上游 LLM 返回 tool call：

1. 通过当前 `request_id` 找到 `instruction_chain_id`。
2. 将每个 `tool_call_id` 绑定到该 chain。
3. 写入 `chain.ToolCallIDs`。
4. 写入审计链和安全事件 detail。

```text
tool_call call_a
  -> tool_call_id(call_a) -> sg_chain_1
```

### 6.3 tool result 请求

当请求尾部包含 tool result：

1. 从 tool result 中提取 `tool_call_id`。
2. 通过 `tool_call_id -> instruction_chain_id` 找回 chain。
3. 如果找到 chain，则本请求绑定到该 chain。
4. 使用 chain 内保存的上下文做 ShepherdGate 检测。
5. 更新 chain 的上下文和更新时间。

```text
tool_result call_a
  -> call_a -> sg_chain_1
  -> req_2 -> sg_chain_1
```

### 6.4 final result

最终回答通过当前 `request_id` 找到 chain：

```text
final result req_1
  -> request_id(req_1) -> sg_chain_1
```

如果 final result 是 tool result 后续请求产生的，则通过该请求的 `request_id -> chain_id` 关联。

## 7. 对现有状态的改造

### 7.1 streamBuffer

当前：

```go
pp.streamBuffer
```

改为：

```go
pp.requestStates[requestID].StreamBuffer
```

所有流式 chunk 处理必须先通过 `requestIDFromContext(ctx)` 找到 request state，再读写对应 `StreamBuffer`。

禁止继续使用全局 `pp.streamBuffer` 作为当前请求缓冲。

### 7.2 lastContextMessages / lastUserMessageContent

当前：

```go
pp.lastContextMessages
pp.lastUserMessageContent
```

改为：

```go
chain.LastContextMessages
chain.LastUserMessageContent
```

ToolCallResult ReAct 分析必须从 chain 中取上下文。

如果无法找到 chain，则降级使用当前请求 messages 构建上下文，并打印日志：

```text
[ShepherdGate][Flow][chain] chain_not_found; using current request context only
```

### 7.3 pendingRecovery

当前：

```go
pp.pendingRecovery
pp.pendingRecoveryArmed
```

改为：

```go
chain.PendingRecovery
chain.PendingRecoveryArmed
```

确认/取消只作用于当前 chain。

如果无法确定 chain，但用户输入明显是“继续/取消”，不得清理其它 chain 的 pending recovery。

### 7.4 blockedToolCallIDs

当前：

```go
pp.blockedToolCallIDs[tool_call_id] = expiresAt
```

改为：

```go
chain.BlockedToolCallIDs[tool_call_id] = expiresAt
```

同时保留一个全局 `toolCallToChain` 索引用于从 tool result 找 chain。

查询时：

1. 优先通过当前 request 的 chain 查询。
2. 如果当前 request 没有 chain，则通过 `tool_call_id -> chain` 查询。
3. 仍找不到时走降级策略。

## 8. 历史恢复策略

软件重启后，内存中的 chain 会丢失。当前已有从历史 ShepherdGate 模拟消息恢复 pending recovery 的机制，可以扩展为恢复 chain。

恢复规则：

1. 在最新用户消息之前查找最近的 ShepherdGate 确认消息。
2. 向前扫描紧邻的 tool result。
3. 提取 tool result 的 `tool_call_id`。
4. 创建一个恢复型 chain：

```text
chain_id = "sg_recovered_" + request_id
source = "request_history"
```

5. 绑定：

```text
request_id -> recovered_chain_id
tool_call_id -> recovered_chain_id
```

6. 将提取到的 `tool_call_id` 放入该 chain 的 blocked set。

如果历史中没有 tool_call_id：

- 创建临时 chain。
- pendingRecovery 可以恢复。
- blocked tool result 无法精确改写，只能根据当前消息上下文做高风险检测。

## 9. 降级策略

### 9.1 tool result 缺少 tool_call_id

风险：

- 无法通过 tool id 找回用户指令链。

处理：

- 使用当前 request 创建临时 chain。
- 对 tool result 执行 ShepherdGate 检测。
- 若命中高风险，则 `NEEDS_CONFIRMATION`。
- 不写入精确 blocked tool id，只记录 request 级隔离事件。

### 9.2 tool_call_id 冲突

风险：

- 两条并行链路使用相同 tool_call_id。

处理：

- 如果 `tool_call_id` 已绑定到不同未过期 chain，打印冲突日志。
- 不覆盖旧绑定。
- 对新请求生成 synthetic scoped id：

```text
scoped_tool_call_id = request_id + ":" + tool_call_id
```

- 安全事件中记录原始 `tool_call_id` 和 scoped id。

### 9.3 无法恢复 chain

风险：

- 上游压缩历史，缺少 tool call/tool result 配对。

处理：

- 使用当前 request 作为临时 chain。
- 用户输入检测仍强制执行。
- tool result 高风险 fallback 仍执行。
- RequestRewrite 无法精确改写历史时，记录 `chain_recovery_failed` 日志。

### 9.4 多链 pending recovery 同时存在

风险：

- 用户回复“继续”但无法判断对应哪条 chain。

处理：

- 如果当前 request 能通过历史 ShepherdGate 消息或 tool id 定位 chain，只确认该 chain。
- 如果无法定位且存在多个 pending recovery，不执行全局确认。
- 返回提示用户重新明确要继续的操作，或开启新会话。

## 10. 安全事件和审计字段

安全事件 detail 建议新增：

```json
{
  "instruction_chain_id": "sg_chain_xxx",
  "chain_source": "user_input|tool_result|request_history|temporary",
  "chain_degraded": false,
  "chain_degrade_reason": ""
}
```

审计日志建议：

- 保留当前审计 log id。
- 增加 `instruction_chain_id` 字段作为运行时安全链关联 ID。
- `request_id` 仍表示单次 HTTP/LLM 请求。
- `instruction_chain_id` 表示一次用户指令及其 tool call/tool result/final result 生命周期。

关系：

```text
AuditLog.ID: 持久化审计记录 ID
request_id: 单次代理请求 ID
instruction_chain_id: ShepherdGate 运行时安全隔离 ID
```

不要把这三个 ID 混成一个概念。

## 11. 日志要求

新增 stage：

```text
[ShepherdGate][Flow][chain]
```

关键日志：

```text
chain_created: chain_id=... request_id=...
chain_bound_request: chain_id=... request_id=...
chain_bound_tool_call: chain_id=... tool_call_id=...
chain_resolved_from_tool_result: chain_id=... request_id=... tool_call_id=...
chain_recovered_from_history: chain_id=... tool_call_ids=[...]
chain_degraded: reason=missing_tool_call_id request_id=...
chain_conflict: tool_call_id=... existing_chain=... new_chain=...
chain_expired: chain_id=...
```

所有日志必须包含 `request_id`，能拿到 chain 时必须包含 `instruction_chain_id`。

## 12. 实施步骤

### 第一步：新增运行时 chain 管理

新增文件建议：

```text
go_lib/core/proxy/security_chain_state.go
go_lib/core/proxy/security_chain_state_test.go
```

实现：

- 创建 chain。
- 绑定 request。
- 绑定 tool call。
- 通过 request 找 chain。
- 通过 tool call 找 chain。
- TTL 清理。
- 冲突检测。

### 第二步：request state 隔离

新增：

```text
request_id -> RequestRuntimeState
```

将 `StreamBuffer` 从 `ProxyProtection` 全局字段迁移到 request state。

要求：

- `onRequest` 创建 request state。
- `onResponse` / `onStreamChunk` 通过 request id 获取 request state。
- 请求结束后清理 request state。

### 第三步：上下文迁移到 chain

迁移：

- `lastContextMessages`
- `lastUserMessageContent`

ToolCallResult 分析改为从 chain 读取上下文。

### 第四步：pending recovery 按 chain 隔离

迁移：

- `pendingRecovery`
- `pendingRecoveryArmed`

确认、取消、历史恢复都必须传入或解析出 chain。

### 第五步：blocked tool call 按 chain 隔离

迁移：

- `blockedToolCallIDs`

RequestRewrite 改为：

1. 解析当前 request chain。
2. 对每个 tool message 查询 chain 内 blocked id。
3. 找不到当前 chain 时，再通过 `tool_call_id -> chain` 兜底。

### 第六步：审计和安全事件补字段

将 `instruction_chain_id` 写入：

- security event detail
- TruthRecord 或其扩展字段
- AuditLog
- 关键 monitor log

### 第七步：测试

必须补充测试：

- 两个并行 request 各自有独立 stream buffer。
- 两个并行用户输入分别触发 pending recovery，不互相覆盖。
- A 链 blocked tool result 不会改写 B 链同名或不同名 tool result。
- tool result 通过 `tool_call_id` 找回正确 chain。
- 软件重启后从历史 ShepherdGate 消息恢复 chain。
- 缺少 tool_call_id 时进入降级路径。
- 多 pending recovery 无法定位时不做全局确认。

## 13. 验收标准

实现完成后必须满足：

- 单会话串行现有功能不回退。
- 并行请求下 `streamBuffer` 不串。
- 并行请求下 `ToolCallResult` 不读取其它请求的上下文。
- 并行请求下 `pendingRecovery` 不互相覆盖。
- 被阻断 tool result 只在对应 chain 中 rewrite。
- 审计和安全事件可通过 `instruction_chain_id` 串起一条完整用户指令链。
- 降级场景有明确 `[ShepherdGate][Flow][chain]` 日志。

## 14. 最终结论

在通用代理场景下，不能依赖 `agent_id/session_id`。可行方案是由代理自己生成 `instruction_chain_id`，并用 `request_id`、`tool_call_id` 建立运行时关联。

这套机制不是完美的：当上游缺少 tool_call_id、复用 tool_call_id 或压缩历史时，仍然只能降级处理。但它可以显著降低并行用户输入和多智能体架构下的状态串扰风险，并尽可能保证 ShepherdGate 的安全检查机制持续生效。
