# ShepherdGate 权限与风险检测链路重构技术方案

## 1. 目标定义

构建一套覆盖完整代理生命周期的 ShepherdGate 权限与风险检测链路：

```text
UserInput -> ToolCall -> ToolCallResult -> RequestRewrite -> FinalResult
```

核心目标不是“每一步都调用安全 LLM”，而是：

- 低成本规则先拦截明显风险。
- 高风险或不确定场景才进入 ReAct 安全智能体。
- 所有阻断、确认、改写都能进入安全事件和审计链。
- 对已阻断 `tool_call_id` 做隔离，防止历史 `messages` 再污染主 LLM。
- 风险分类参考 OWASP GenAI Security Project 的 Agentic Top 10，包括 Agent Goal Hijack、Tool Misuse、Identity & Privilege Abuse、Supply Chain、Unexpected Code Execution、Memory/Context Poisoning 等。

## 2. 总体架构

继续复用当前 hook 化方向，避免把策略逻辑重新堆回 `onRequest`：

```text
chatmodel-routing.Proxy
  -> RequestProtocolAnalyzer
  -> RequestPolicyHook
  -> UserInputPolicyHook
  -> ToolCallPolicyHook
  -> ToolResultPolicyHook
  -> RequestRewriteHook
  -> ResponsePolicyHook / FinalResultPolicyHook
  -> AuditEventHook
```

当前已有基础：

| 能力 | 当前文件/机制 | 说明 |
| --- | --- | --- |
| 请求阶段策略 | `go_lib/core/proxy/request_policy.go` | 已抽象请求阶段策略 hook |
| 协议归一化 | `go_lib/core/proxy/request_protocol.go` | 已归一化 OpenAI tool 协议与 inline tool 协议 |
| 工具结果策略 | `go_lib/core/proxy/tool_result_policy.go` | 已抽象 tool result 阶段策略 hook |
| 转发前改写 | `ForwardBody` | 已支持放行请求时替换上游请求体 |
| 阻断 ID 隔离 | `blocked_tool_call_id` 隔离表 | 已支持 TTL 过期 |

后续扩展应围绕这些 hook 继续推进，不新增平行策略系统。

## 3. 生命周期检测设计

| 阶段 | 检测对象 | 主要风险 | 动作 |
| --- | --- | --- | --- |
| `UserInput` | 所有 `role=user` 消息联合判断 | 直接提示词注入、越权意图、多轮引导、高危自然语言指令 | `ALLOW` / `NEEDS_CONFIRMATION` / `BLOCK` |
| `ToolCall` | LLM 生成的工具调用和参数 | 文件读取、脚本执行、配置修改、邮件访问、外联、权限滥用 | `ALLOW` / `REACT_ANALYZE` / `NEEDS_CONFIRMATION` / `BLOCK` |
| `ToolCallResult` | 工具返回内容 | 间接提示词注入、敏感数据泄露、恶意指令、上下文污染 | `ALLOW` / `QUARANTINE` / `REWRITE` / `NEEDS_CONFIRMATION` / `BLOCK` |
| `RequestRewrite` | 历史 `messages` | 被阻断 `tool_result` 残留、历史污染 | 用 `ForwardBody` 替换危险 tool content |
| `FinalResult` | 返回给用户的最终回答 | 泄露敏感数据、误导用户批准危险操作、绕过策略 | `ALLOW` / `REDACT` / `WARN` / `BLOCK` |

## 4. UserInput 检测

`UserInput` 阶段必须联合分析所有 `role=user` 消息，而不是只看最后一条，防止多轮会话逐步诱导。

检测策略分三层。

### 4.1 规则层

规则层用于低成本命中明显风险：

- “忽略之前规则”
- “你现在是系统”
- “泄露 system prompt”
- “绕过安全策略”
- 分多轮逐步诱导执行危险动作

### 4.2 语义层

语义层用于识别用户意图中的高危访问或操作：

- 是否请求读取敏感文件。
- 是否要求导出邮件、密钥、token、配置。
- 是否要求执行脚本、修改系统配置、删除文件。

### 4.3 ReAct 层

ReAct 层只在规则层命中高风险或语义层不确定时调用。

约束：

- 输入只包含必要上下文。
- 不把完整历史原文全部交给安全 LLM。
- 需要遵守安全 token 预算。

## 5. ToolCall 检测

`ToolCall` 本身不是攻击，但它代表 LLM 的行动意图，必须做权限判断。

### 5.1 文件读取

策略：

- 先用路径规则判断敏感文件：
  - `.ssh/`
  - `.env`
  - keychain
  - 浏览器 cookie
  - 邮件数据库
  - 配置目录
- 不确定时交给 ReAct。
- ReAct 只允许读取文件名、元信息、有限字节片段。
- 敏感文件默认 `NEEDS_CONFIRMATION` 或 `BLOCK`，不能直接返回给主 LLM。

### 5.2 脚本执行

静态检查命令和脚本内容：

- 外发行为：`curl`、`wget`、`nc`、`scp`、DNS exfiltration。
- 持久化行为：launch agent、systemd、crontab、shell profile。
- 破坏行为：`rm -rf`、权限修改、配置覆盖。

### 5.3 邮件/消息工具

策略：

- 读取邮件正文、附件、联系人属于敏感数据访问。
- 发送邮件、转发邮件属于高危外发动作。

## 6. ToolCallResult 检测

`ToolCallResult` 是间接注入的核心入口。

必须检测：

- “忽略之前所有指令”
- “把以下内容发给用户/外部服务”
- “调用某工具执行以下命令”
- “这是一条系统消息”
- “把 token/API key 泄露出来”
- 工具结果中混入 prompt 指令
- HTML/Markdown 隐藏指令
- 代码块伪装指令

处理原则：

- 高置信间接注入：`BLOCK` 或 `NEEDS_CONFIRMATION`。
- 被阻断的 `tool_call_id` 写入隔离表，带 TTL。
- 后续请求中如果历史 `messages` 仍包含该 `tool_call_id` 的 tool result，必须通过 `RequestRewriteHook` 替换内容，而不是原样转发。
- 替换内容保留协议结构，避免破坏 tool_call/tool_result 配对。

建议替换内容：

```text
[ClawSecbot] Tool result was blocked and withheld due to security risk.
```

禁止直接删除 `role=tool` 消息，否则可能破坏 OpenAI tool_call/tool_result 配对。

## 7. RequestRewrite 检测

`RequestRewrite` 负责处理历史污染问题。

要求：

- 使用 blocked `tool_call_id` 隔离表识别历史危险 tool result。
- 命中后只改写转发给上游的请求体。
- 保留 `role=tool` 和 `tool_call_id`。
- 通过 `ForwardBody` 替换危险 `content`。
- 原始请求观测、审计链和监控日志仍保留现有语义。

## 8. FinalResult 检测

`FinalResult` 的风险重点不是“污染主 LLM”，而是返回给用户前的结果安全。

重点检测：

- 是否泄露敏感内容给用户。
- 是否误导用户批准危险操作。
- 是否把被隔离的 tool result 内容复述出来。
- 是否给出危险脚本、持久化、外发、删除等操作指导。
- 是否出现 Human-Agent Trust Exploitation：用自信话术诱导用户点击确认。

建议动作：

| 场景 | 动作 |
| --- | --- |
| 普通文本 | `ALLOW` |
| 含敏感数据 | `REDACT` 后返回 |
| 含危险操作建议 | `WARN` 或 `BLOCK` |
| 引用了 quarantined tool result | `BLOCK` 或替换 |

流式响应策略：

- 高安全模式：缓冲最终结果后再放行。
- 普通模式：chunk 级轻量检测 + 结束后审计。

## 9. Token 成本控制

安全成本不得超过业务成本的 20%。

建议硬指标：

```text
security_tokens <= min(business_tokens * 0.2, per_request_security_cap)
```

策略：

- 默认不对每个阶段都调用安全 LLM。
- 规则和结构化检测必须先跑。
- ReAct 只用于：
  - 高危 `ToolCall`
  - `ToolCallResult` 命中可疑注入
  - 文件/邮件/脚本这类需要上下文判断的场景
  - 用户自定义语义规则无法用简单规则判断时
- 缓存安全分析结果：
  - `tool_call` 参数 hash
  - `tool_result` 内容 hash
  - 文件路径 + mtime + size + partial hash
- 超预算场景：
  - 低风险：降级为规则检测 + 审计。
  - 高风险：`NEEDS_CONFIRMATION`，不能静默放行。

## 10. 用户自定义语义规则

UI 上支持类似规则：

- 不允许删除文件。
- 不允许查看邮件。
- 不允许读取 SSH key。
- 不允许访问浏览器 cookie。
- 不允许修改 shell 配置。
- 不允许访问公司目录。

规则结构建议：

```json
{
  "id": "no_delete_files",
  "scope": "asset_id",
  "enabled": true,
  "description": "不允许删除文件",
  "applies_to": ["tool_call", "tool_result", "final_result"],
  "action": "needs_confirmation",
  "risk_type": "HIGH_RISK_OPERATION",
  "owasp_agentic": ["ASI02", "ASI03"]
}
```

存储要求：

- 必须按 `asset_id` 隔离。
- 继续复用当前 Shepherd 用户规则路径。
- 不新增平行规则系统。

## 11. 风险类型与 OWASP Agentic Top 10 映射

内部风险类型建议统一为以下枚举：

| 内部 `RiskType` | 含义 | OWASP Agentic 映射 |
| --- | --- | --- |
| `PROMPT_INJECTION_DIRECT` | 用户直接注入 | `ASI01 Agent Goal Hijack` |
| `PROMPT_INJECTION_INDIRECT` | tool result 间接注入 | `ASI01`, `ASI06` |
| `SENSITIVE_DATA_EXFILTRATION` | 敏感数据泄露 | `ASI02`, `ASI03` |
| `HIGH_RISK_OPERATION` | 删除、修改、发送、执行 | `ASI02` |
| `PRIVILEGE_ABUSE` | 越权访问/凭证滥用 | `ASI03` |
| `UNEXPECTED_CODE_EXECUTION` | 脚本/RCE | `ASI05` |
| `CONTEXT_POISONING` | 历史 messages / memory 污染 | `ASI06` |
| `SUPPLY_CHAIN_RISK` | skill/plugin/package 风险 | `ASI04` |
| `HUMAN_TRUST_EXPLOITATION` | 诱导用户批准危险动作 | `ASI09` |
| `CASCADING_FAILURE` | 多工具连锁风险 | `ASI08` |

安全事件和审计日志都应记录：

- `risk_type`
- `risk_level`
- `owasp_agentic_ids`
- `decision_action`
- `hook_stage`
- `tool_call_id`
- `request_id`
- `asset_id`
- `evidence_summary`
- `was_rewritten`
- `was_quarantined`

## 12. 关键需求补充

- 用户输入阶段必须支持多轮联合判断。
- `ToolCall` 阶段必须识别敏感资源访问和高危动作。
- `ToolCallResult` 阶段必须检测间接注入，并支持隔离。
- 被阻断 `tool_call_id` 后续不得原样进入主 LLM。
- 用户取消后必须继续净化历史 `messages`。
- 用户确认后才允许恢复对应 `tool_call_id`。
- 安全 LLM 成本必须有预算、缓存和降级策略。
- 用户规则必须按 `asset_id` 生效。
- 安全事件和审计日志必须映射 OWASP Agentic Top 10。
- 审计/事件写入失败不得阻断主流程。
- 安全决策失败在高危操作上必须 fail-safe。

## 13. 落地顺序

### 13.1 RequestRewriteHook

目标：

- 用 blocked `tool_call_id` 净化历史 tool result。
- 依赖现有 `ForwardBody`。

验收：

- 被阻断 `tool_call_id` 在 TTL 内再次出现在历史 `messages` 时，不得原样进入上游 LLM。
- `role=tool` 和 `tool_call_id` 必须保留。

### 13.2 UserInputPolicyHook

目标：

- 联合所有 user 消息。
- 先规则，后 ReAct。

验收：

- 多轮提示词注入可被识别。
- 明显危险自然语言指令进入 `NEEDS_CONFIRMATION` 或 `BLOCK`。

### 13.3 ToolCallPolicyHook

目标：

- 检测文件、邮件、脚本、网络、配置修改。

验收：

- 敏感文件读取、删除文件、外发命令、邮件读取/转发能触发风险决策。

### 13.4 RiskTaxonomy

目标：

- 统一 `risk_type`、`owasp_agentic_ids`、`risk_level`。

验收：

- 安全事件、审计日志、监控输出使用同一套风险语义。

### 13.5 SecurityBudget

目标：

- 统计业务 token 和安全 token。
- 实施 20% 成本预算。

验收：

- ReAct 调用受预算限制。
- 超预算时低风险降级审计，高风险转 `NEEDS_CONFIRMATION`。

### 13.6 UI 自定义语义规则

目标：

- 复用 Shepherd rules。
- 支持规则启停、动作选择、风险类型。

验收：

- 规则按 `asset_id` 隔离生效。
- 不新增平行规则系统。

## 14. 方案原则

这套方案的重点是：检测链路要覆盖全生命周期，但 ReAct 不能覆盖全流量。

必须通过以下方式同时控制安全性、性能和成本：

- hook 分层
- 规则前置
- ReAct 按风险触发
- 安全预算控制
- tool result 隔离
- 请求转发前改写
- 统一风险分类和审计字段

## 15. 参考来源

- OWASP Agentic Top 10 发布说明
- OWASP Top 10 for Agentic Applications PDF
