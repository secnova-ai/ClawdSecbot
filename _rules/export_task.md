// 状态类数据 来自主页+防护窗口 记录到指定文件export/status.json
{
  "botInfo": [
    {
      "name": "openclaw",
      "id": "xxxxxxxxxxxxxxxxxx",
      "pid": "12345",
      "image": "openclaw-gateway",      
      "conf": "/Users/hudy/.openclaw/openclaw.json",
      "bind": "127.0.0.1:18789",
      "protection": "enabled",
      "botModel": {
        "provider": "openai",
        "id": "glm-4.5",
        "url": "xxx",
        "key": "xxx"
      },
      "metrics": {
        "analysisCount": 33,
        "messageCount": 54,
        "warningCount": 3,
        "blockCount": 3,
        "totalToken": 228632,
        "inputToken": 220799,
        "outputToken": 7833,
        "protectionTotalToken": 87801,
        "protectionInputToken": 72243,
        "protectionOutputToken": 15558,
        "toolCallCount": 35
      }
    }
  ],
  "riskInfo":[
    {
      "name": "检测到 MCP Server 未配置认证机制",
      "level": "high",
      "source": "openclaw",
      "botId": "xxx",
      "mitigation": [
        {
          "desc": "",
          "command": ""        
        }
      ]
    }
  ],
  "skillResult":[
    {
      "name": "skillName",
      "level": "high",
      "source": "/Users/hudy/.openclaw/workspace/skills/skillName",
      "botId": "xxx",
      "issue": [
        {
          "type": "prompt_injection",
          "desc": "Skill 包含可注入的提示模板",
          "evidence": "prompt = f'Execute {user_input}'"        
        }
      ]
    }
  ],
  "securityModel": {
    "provider": "openai",
    "id": "glm-5",
    "url": "xxx",
    "key": "xxx"
  },
  "timestamp": 1773488031881
}

// 记录类数据 来自审计窗口 追加记录到指定文件export/audit.jsonl，一行一个json对象
{
    "botId": "xxx",
    "logId": "audit_1773489418996224000_3",
    "logTimestamp": "2026-03-14 19:56:58",
    "requestId": "req_1773489418996217000_57",
    "model": "MiniMax-M2.1",
    "action": "已允许",
    "riskLevel": "",
    "riskCauses": "",
    "durationMs": 53187,
    "tokenCount": 657,
    "userRequest": "[Sat 2026-03-14 19:56 GMT+8] 今天天气怎么样",
    "toolCallCount": 2,
    "toolCalls": [
        {
            "tool": "read",
            "parameters": "{}",
            "result": "Shanghai: ⛅️ +8°C\n"
        },
        {
            "tool": "exec",
            "parameters": "{\"command\":\"curl -s \"wttr.in/Shanghai\"}",
            "result": ""
        }
    ]
}

// 记录类数据 来自防护监控窗口的安全事件 追加记录到指定文件export/events.jsonl，一行一个json对象
{
  "botId": "xxx",
  "eventId": "String",          # 事件ID
  "timestamp": "DateTime",      # 触发事件
  "event_type": "String",       # 工具执行/阻断
  "action_desc": "String",      # 工具执行动作描述
  "risk_type": "String",        # 风险类型
  "detail": "String",           # 补充细节
  "source": "String"            # 来源
}