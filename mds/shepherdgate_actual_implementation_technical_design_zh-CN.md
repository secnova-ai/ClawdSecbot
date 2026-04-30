# ShepherdGate 全链路安全检测实现说明

## 1. 文档定位

本文按当前 `shepherdgate-refact` 分支的实际代码实现编写，用于说明 ShepherdGate 在代理请求链路中的真实落点、检测阶段、风险事件、日志、token 统计和可扩展点。

当前实现目标是：

- 覆盖 `UserInput -> ToolCall -> ToolCallResult -> RequestRewrite -> FinalResult` 全链路。
- 对直接提示词注入、间接提示词注入、敏感数据外泄、高危操作、上下文污染等风险进行检测。
- 对被阻断的 `tool_call_id` 做隔离和历史恢复，避免 OpenAI `messages` 历史数组把污染内容再次送入主 LLM。
- 保留 hook 结构，后续新增检测能力时优先追加 hook 或扩展现有策略，不把逻辑重新堆回主处理函数。
- 将安全事件和审计日志关联到统一风险类型，并映射 OWASP Agentic Top 10 编号。

## 2. 核心文件

| 模块 | 文件 | 职责 |
| --- | --- | --- |
| 请求主链路 | `go_lib/core/proxy/proxy_protection_handler.go` | 串联请求解析、各阶段 hook、审计和监控事件 |
| 用户输入检测 | `go_lib/core/proxy/user_input_policy.go` | 联合分析全部 `role=user` 消息，并调用安全 LLM 语义检测 |
| 工具调用检测 | `go_lib/core/proxy/tool_call_policy.go` | 调用 ShepherdGate ReAct/LLM 分析 tool call 名称和参数 |
| 工具结果检测 | `go_lib/core/proxy/tool_result_policy.go` | 对 tool result 做 ReAct/LLM 安全分析、隔离和模拟返回 |
| 历史污染改写 | `go_lib/core/proxy/request_rewrite_policy.go` | 改写历史中已隔离的 tool result content |
| 最终结果检测 | `go_lib/core/proxy/final_result_policy.go` | 返回用户前检测危险建议、敏感数据和隔离内容复述 |
| 风险决策封装 | `go_lib/core/proxy/security_policy_decision.go` | 统一阻断、确认、模拟返回、审计和监控事件写入 |
| 风险分类 | `go_lib/core/proxy/risk_taxonomy.go` | 定义风险枚举、hook 阶段和 OWASP Agentic ID 映射 |
| 敏感证据脱敏 | `go_lib/core/proxy/security_evidence_redaction.go` | 统一清理审计 evidence 中的 token/secret/key |
| 安全流程日志 | `go_lib/core/proxy/security_flow_log.go` | 统一 `[ShepherdGate][Flow]` 日志前缀 |
| tool_call 隔离 | `go_lib/core/proxy/blocked_tool_call.go` | 维护被阻断 `tool_call_id` 的内存 TTL |
| 确认/恢复机制 | `go_lib/core/proxy/shepherd_recovery.go` | 识别用户确认/取消，并从历史 `messages` 恢复隔离状态 |
| 安全 LLM | `go_lib/core/shepherd/shepherd_gate.go` | 用户输入语义检测、工具结果 ReAct 检测、国际化模拟回复 |
| ReAct 分析器 | `go_lib/core/shepherd/shepherd_react_analyzer.go` | 工具调用/工具结果上下文的安全智能体分析 |
| 用户规则持久化 | `go_lib/core/service/protection_service.go`、`go_lib/core/repository/protection_repository.go` | 保存和读取用户自定义语义规则 |
| UI 风险标签 | `lib/utils/security_event_labels.dart` | 将风险枚举转为界面展示文案 |

## 3. 请求生命周期

当前请求主链路在 `ProxyProtection.onRequest` 中执行。流程如下：

```text
Client Request
  -> request policy
  -> protocol analyzer
  -> recovery detection
  -> user_input hook
  -> tool_call_result hook
  -> request_rewrite hook
  -> upstream LLM
  -> tool_call hook
  -> final_result hook
  -> Client Response
```

实际执行顺序要区分请求方向和响应方向：

1. 请求进入后先建立审计 request id、TruthRecord、监控事件。
2. `analyzeRequestProtocol` 解析 OpenAI tool 协议和内嵌 tool 协议。
3. 从历史消息中恢复可能丢失的 ShepherdGate 待确认状态。
4. 若本轮不是确认/取消操作，则执行 `UserInputPolicyHook`。
5. 若请求尾部存在 tool result，则执行 `ToolResultPolicyHook`。
6. 对已隔离的历史 tool result 执行 `RequestRewriteHook`，只改写转发给上游 LLM 的 `ForwardBody`。
7. 上游 LLM 返回 tool call 时执行 `ToolCallPolicyHook`。
8. 上游 LLM 返回最终文本时执行 `FinalResultPolicyHook`。

## 4. UserInput 检测

实现文件：`go_lib/core/proxy/user_input_policy.go`、`go_lib/core/shepherd/shepherd_gate.go`

### 4.1 检测对象

`UserInputPolicyHook` 不只看最后一条用户消息，而是通过 `collectUserInputText` 联合分析全部 `role=user` 消息，防止多轮逐步诱导。

同时会跳过 OpenClaw 注入的 memory/user context 类内容，避免把系统补充的历史检索上下文误判为用户真实意图。

### 4.2 安全 LLM 语义检测

用户输入阶段会调用 `ShepherdGate.CheckUserInput` 做语义分类。不再用本地静态规则替代安全模型判定，也不会因为 token 成本预算跳过检测。

安全 LLM 的提示词已按不可信输入隔离：

- system prompt 明确说明待分类内容是 untrusted data。
- 要求只分类，不执行、不服从输入中的任何指令。
- 用户内容被包装到 JSON payload 的 `untrusted_user_content` 字段中。
- 使用 `BEGIN_UNTRUSTED_USER_INPUT_JSON` / `END_UNTRUSTED_USER_INPUT_JSON` 定界。
- `reason` 和 `action_desc` 按 ShepherdGate 当前语言返回，`risk_type` 保持枚举值。

直接提示词注入会直接 `BLOCK`，并在模拟返回中提示用户开启新会话：

```text
请开启新的会话恢复对话，本轮会话已被污染，继续对话将被拦截。
```

英文环境下返回对应英文提示。

## 5. ToolCall 检测

实现文件：`go_lib/core/proxy/tool_call_policy.go`

`ToolCall` 是 LLM 主动生成的行动意图，不等同于攻击本身，但必须做权限判断。当前实现检查工具名和参数文本，命中后返回 `NEEDS_CONFIRMATION`。

当前实现会把 tool call 转为 `ToolCallInfo`，交给 `ShepherdGate.CheckToolCall` 的 ReAct/LLM 分析器判断。工具验证器只提供敏感工具元数据，不再作为本地静态阻断规则替代安全模型。

ReAct 输入包括：

| 规则 | 风险类型 | 示例 |
| --- | --- | --- |
| 敏感路径访问 | `SENSITIVE_DATA_EXFILTRATION` | `.ssh/id_rsa`、`/etc/shadow`、`.env`、keychain、cookie、邮件 |
| 破坏性操作 | `HIGH_RISK_OPERATION` | `rm -rf`、delete、remove、wipe、format |
| 外发动作 | `SENSITIVE_DATA_EXFILTRATION` | `curl`、`wget`、`nc`、`scp`、HTTP URL、发送到邮箱 |
| 持久化/配置修改 | `PRIVILEGE_ABUSE` | crontab、launch agent、systemd、shell profile |
| 脚本执行 | `UNEXPECTED_CODE_EXECUTION` | `sh -c`、`python -c`、`node -e`、powershell、osascript |

ToolCall evidence 会先经过 `redactSecurityEvidence`，避免参数中出现的 token/secret 被写入安全事件 detail。

## 6. ToolCallResult 检测

实现文件：`go_lib/core/proxy/tool_result_policy.go`、`go_lib/core/shepherd/shepherd_react_analyzer.go`

`ToolCallResult` 是间接提示词注入的主要入口。当前实现只在请求中存在 tool result 时触发。

### 6.1 沙箱阻断结果识别

如果 tool result 已经是 ClawdSecbot 沙箱阻断结果：

- 记录 `SANDBOX_BLOCKED` 安全事件。
- 标记对应 `tool_call_id` 为 blocked。
- 不重复弹出用户确认。

### 6.2 ReAct 安全分析

普通 tool result 会进入 ShepherdGate ReAct 分析：

- 上下文：最近消息、最新 assistant tool calls、tool results、最后用户消息。
- 安全模型：来自 ShepherdGate security model 配置。
- 输出：`allowed`、`reason`、`risk_level`、`confidence`、`action_desc`、`risk_type`、`skill`、`usage`。

如果 ReAct 返回 `ALLOWED`，请求继续转发。

如果返回 `NEEDS_CONFIRMATION` 或阻断类决策：

- 当前 tool result 对应的 `tool_call_id` 会写入 blocked id 内存表。
- 记录 pending recovery，等待用户回复确认或取消。
- 返回 ShepherdGate 模拟消息给用户。
- 写入监控事件、安全事件和审计链。

### 6.3 无预算降级

ToolCallResult 阶段始终调用 ShepherdGate ReAct/LLM 分析。安全 token 会被统计到 `auditTokens`、`auditPromptTokens`、`auditCompletionTokens`，但不会作为跳过检测或切换静态 fallback 的条件。

## 7. 被阻断 tool_call_id 隔离

实现文件：`go_lib/core/proxy/blocked_tool_call.go`

当前实现维护一个内存级 blocked `tool_call_id` 表：

- 默认 TTL：2 小时。
- 写入时会清理过期项。
- 查询时也会清理过期项。
- 用户确认后会清理对应 blocked id。

隔离的目的不是修改原始历史，而是在后续请求转发上游 LLM 前识别危险历史 tool result，并用安全占位内容替换。

## 8. 历史恢复机制

实现文件：`go_lib/core/proxy/shepherd_recovery.go`

仅靠内存 blocked id 不足以覆盖软件重启场景，因为 OpenClaw 后续请求仍可能携带历史 `messages`。因此实现了从历史消息恢复 pending recovery 的机制：

1. 在最新用户消息之前查找最近的 ShepherdGate 模拟确认消息。
2. 如果找到确认消息，向前扫描紧邻的 `role=tool` 消息。
3. 提取这些 tool 消息的 `tool_call_id`。
4. 重建 pending recovery，并把这些 id 重新标记为 blocked。
5. 再通过 `EvaluateRecoveryIntent` 判断最新用户消息是确认、取消还是无明确意图。

用户确认后：

- 若本轮需要注入 tool result，则跳过 tool result 安全检测并清理 blocked id。
- 若只是历史确认恢复，没有尾部 tool result，则清理 pending recovery 并允许请求继续。

用户取消后：

- 清理 pending recovery。
- 保持历史 tool result 的隔离状态，后续通过 request rewrite 避免污染主 LLM。

## 9. RequestRewrite 历史污染处理

实现文件：`go_lib/core/proxy/request_rewrite_policy.go`

OpenAI tool 协议要求 assistant tool call 和 tool result 配对，因此不能直接删除历史 `role=tool` 消息。

当前实现做法：

- 遍历请求 `messages`。
- 找到 `role=tool` 且 `tool_call_id` 在 blocked id 表中的消息。
- 使用 `sjson.SetBytes` 只替换 `messages[i].content`。
- 保留 `role=tool`、`tool_call_id` 和消息顺序。
- 通过 `FilterRequestResult.ForwardBody` 只改写发给上游 LLM 的请求体。

占位内容：

```text
[ClawdSecbot] This tool result was quarantined by security policy because its tool_call_id was previously blocked. The original content is withheld.
```

改写会产生 `CONTEXT_POISONING` 风险事件，`decision_action=REWRITE`，并记录 `was_rewritten=true`。

## 10. FinalResult 检测

实现文件：`go_lib/core/proxy/final_result_policy.go`

FinalResult 是返回用户前的最后一道检查。当前规则包括：

- 如果最终回答引用了隔离 tool result 占位内容，直接 `BLOCK`，风险类型 `CONTEXT_POISONING`。
- 如果命中用户自定义 final result 语义规则，按规则 action 执行。
- 如果包含危险操作建议，直接 `BLOCK`，风险类型 `HIGH_RISK_OPERATION`。
- 如果诱导用户无脑确认安全提示，直接 `BLOCK`，风险类型 `HUMAN_TRUST_EXPLOITATION`。
- 如果包含 API key、GitHub token、Slack token、AWS access key、credential assignment，则执行 `REDACT` 后返回。

FinalResult 的安全事件同样会走 `recordFinalResultPolicyEvent`，并把敏感 evidence 脱敏后写入 detail。

## 11. 用户自定义语义规则

实现文件：

- `go_lib/core/shepherd/shepherd_gate.go`
- `go_lib/core/shepherd/shepherd_user_rules_bundle.go`
- `go_lib/core/service/protection_service.go`
- `go_lib/core/repository/protection_repository.go`
- `lib/services/protection_database_service.dart`
- `lib/widgets/protection_config_dialog.dart`

规则结构：

```json
{
  "semantic_rules": [
    {
      "id": "confirm-delete",
      "scope": "asset",
      "enabled": true,
      "description": "不允许删除文件",
      "applies_to": ["tool_call", "tool_call_result", "final_result"],
      "action": "needs_confirmation",
      "risk_type": "HIGH_RISK_OPERATION",
      "owasp_agentic": ["ASI02"]
    }
  ]
}
```

当前规则支持：

- 按 asset 保存。
- 启动防护时按 `asset_id` 加载一次。
- 保存后热更新运行中的 ShepherdGate。
- 适用于 `tool_call` 和 `final_result`，其中 `tool_call_result` 主要由 ReAct 上下文承接。

规则匹配并不是完整语义向量检索，而是规则描述和目标文本的结构化启发式匹配。当前支持的启发式包括 delete/remove、邮件、SSH key、cookie、配置等关键词族。

## 12. 风险类型与 OWASP Agentic Top 10 映射

实现文件：`go_lib/core/proxy/risk_taxonomy.go`

| 风险类型 | OWASP Agentic ID |
| --- | --- |
| `PROMPT_INJECTION_DIRECT` | `ASI01` |
| `PROMPT_INJECTION_INDIRECT` | `ASI01`, `ASI06` |
| `SENSITIVE_DATA_EXFILTRATION` | `ASI02`, `ASI03` |
| `HIGH_RISK_OPERATION` | `ASI02` |
| `PRIVILEGE_ABUSE` | `ASI03` |
| `UNEXPECTED_CODE_EXECUTION` | `ASI05` |
| `CONTEXT_POISONING` | `ASI06` |
| `SUPPLY_CHAIN_RISK` | `ASI04` |
| `HUMAN_TRUST_EXPLOITATION` | `ASI09` |
| `CASCADING_FAILURE` | `ASI08` |

安全事件 detail 统一包含：

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
- `reason`

`evidence_summary` 在 JSON 序列化前会统一脱敏。

## 13. 国际化

当前 ShepherdGate 使用全局语言设置：

- `ShepherdGate.EffectiveLanguage`
- `skillscan.GetLanguageFromAppSettings`
- `NormalizeShepherdLanguage`

已国际化的内容包括：

- 安全 LLM 的 `reason` 和 `action_desc` 语言约束。
- ShepherdGate 模拟返回的状态、原因、动作、风险类型。
- 直接提示词注入后的“新开会话”污染提示。
- UI 安全事件风险标签。

风险枚举本身仍保留在 event detail 中，供审计和机器处理使用；界面展示使用 `lib/utils/security_event_labels.dart` 映射为用户语言文案。

## 14. 安全事件、审计和监控

当前阻断/确认/改写会同时进入：

- 终端安全流程日志。
- 监控面板事件。
- security event。
- TruthRecord。
- AuditChainTracker。

请求阶段阻断通过 `applyRequestSecurityPolicyDecision` 统一处理。

响应阶段阻断通过 `applyResponseSecurityPolicyDecision` 统一处理。

FinalResult 的 `REDACT` 属于可继续返回的变更，会记录事件但不终止响应。

## 15. 日志规范

安全检测流程日志统一前缀：

```text
[ShepherdGate][Flow][stage] message
```

当前 stage 包括：

- `request`
- `user_input`
- `tool_call`
- `tool_call_result`
- `request_rewrite`
- `final_result`
- `recovery`
- `quarantine`

典型日志：

```text
[ShepherdGate][Flow][request] message_roles=[system user assistant user]
[ShepherdGate][Flow][user_input] analysis_start: user_message_count=3 combined_chars=5191
[ShepherdGate][Flow][tool_call_result] analysis_start: tool_result_count=1 tools=read_file
[ShepherdGate][Flow][tool_call_result] analysis_token_usage: total=5593 prompt=5143 completion=450
[ShepherdGate][Flow][request_rewrite] rewrote historical blocked tool results before upstream forwarding: count=1 tool_call_ids=[call_x]
```

Go 代码日志保持英文，符合项目规范。

## 16. Token 统计原则

当前安全检测不再做预算降级。所有需要安全模型判断的阶段都应调用 ShepherdGate，并记录实际 token 消耗：

| 阶段 | 安全 LLM 策略 |
| --- | --- |
| `user_input` | 必须执行安全 LLM 语义检测 |
| `tool_call` | 必须执行 ShepherdGate ReAct/LLM 分析 |
| `tool_call_result` | 必须执行 ShepherdGate ReAct/LLM 分析 |
| `request_rewrite` | 纯结构化改写，不调用安全 LLM |
| `final_result` | 规则和脱敏，不调用安全 LLM |

安全模型返回的 usage 会累加到代理指标，日志中通过 `analysis_token_usage` 记录，避免安全服务消耗被低估。

## 17. 当前边界

当前实现仍有明确边界：

- blocked `tool_call_id` 是内存状态，重启后依赖历史 `messages` 中的 ShepherdGate 确认消息自恢复。
- 用户自定义语义规则当前是启发式匹配，不是完整 LLM 规则推理。
- FinalResult 当前主要是规则检测和敏感数据脱敏，尚未接入 ReAct。
- 审计 detail 会保留风险枚举和 OWASP ID，用户界面需要通过标签映射展示，不应直接展示枚举。

## 18. 回归测试覆盖

核心测试文件：

- `go_lib/core/proxy/security_policy_hooks_test.go`
- `go_lib/core/proxy/shepherd_llm_policy_test.go`
- `go_lib/core/proxy/shepherd_recovery_test.go`
- `go_lib/core/proxy/security_flow_log_test.go`
- `go_lib/core/shepherd/shepherd_gate_recovery_intent_test.go`
- `go_lib/core/shepherd/shepherd_react_analyzer_test.go`

重点覆盖：

- 直接提示词注入阻断。
- 中文系统提示词绕过识别。
- 冷启动场景 user input 仍调用安全 LLM。
- ToolCall 和 ToolCallResult 始终进入 ShepherdGate ReAct/LLM 分析。
- 用户输入确认不死循环。
- 历史 blocked tool result request rewrite。
- 重启后从历史 ShepherdGate 消息恢复隔离状态。
- 用户确认/取消语义识别。
- final result 危险内容阻断和敏感数据脱敏。
- 风险事件 OWASP ID 和 evidence 脱敏。

## 19. 后续扩展建议

后续新增能力建议遵循以下方式：

1. 新检测点优先作为 hook 增量接入。
2. 新风险类型先扩展 `risk_taxonomy.go`，再补 UI 标签映射。
3. 涉及用户可见文案必须同时处理中英文。
4. 涉及 evidence 必须复用 `redactSecurityEvidence`。
5. 涉及安全 LLM 的 prompt 必须把被检测内容作为不可信数据隔离。
6. 涉及 OpenAI tool 协议历史消息时，优先改写 content，不删除 tool 消息。
7. 新增 Go 业务逻辑必须补对应单测。
