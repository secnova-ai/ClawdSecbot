# ClawdSecbot Web RPC 接口说明

本文档面向通过 `botsec_webd.exe` 集成 ClawdSecbot Web 的安装包或外部程序。

## 基础地址

Web 安装包启动后，`ClawdSecbotWebLauncher.exe` 会启动本机后端：

```text
http://127.0.0.1:<port>/
```

默认优先使用 `18080`。如果端口被其他服务占用，启动器会自动选择空闲端口，并在安装目录写入：

```text
clawdsecbot-web.runtime
```

文件内容包含：

```text
pid=<backend pid>
url=http://127.0.0.1:<port>/
```

健康检查：

```http
GET /health
```

成功响应包含：

```json
{"success": true}
```

## 鉴权流程

首次启动或工作区初始化：

```http
POST /api/v1/bootstrap/init
Content-Type: application/json
```

请求体示例：

```json
{
  "workspace_dir": "[WORKSPACE_DIR]",
  "home_dir": "[HOME_DIR]",
  "sandbox_dir": "[SANDBOX_DIR]",
  "current_version": "1.0.4"
}
```

登录：

```http
POST /api/v1/auth/login
Content-Type: application/json
```

请求体：

```json
{
  "username": "sysadmin",
  "password": "[PASSWORD]"
}
```

后续 RPC 请求需要携带：

```http
Authorization: Bearer [TOKEN]
```

## RPC 调用格式

FFI 方法通过 HTTP RPC 暴露：

```http
POST /api/v1/rpc/<MethodName>
Content-Type: application/json
Authorization: Bearer [TOKEN]
```

统一请求体：

```json
{
  "strings": [],
  "ints": []
}
```

字符串参数按顺序放入 `strings`，整数参数按顺序放入 `ints`。

## Bot 模型参数

保存某个资产实例的 Bot 转发模型参数：

```http
POST /api/v1/rpc/SaveBotModelConfigFFI
```

请求体：

```json
{
  "strings": [
    "{\"asset_name\":\"readyclaw\",\"asset_id\":\"readyclaw:[INSTANCE_ID]\",\"provider\":\"openai\",\"base_url\":\"[MODEL_BASE_URL]\",\"api_key\":\"[API_KEY]\",\"model\":\"[MODEL_NAME]\",\"secret_key\":\"[SECRET_KEY]\"}"
  ],
  "ints": []
}
```

字段说明：

```text
asset_name  可选，资产类型，例如 readyclaw/openclaw。
asset_id    必填，资产实例 ID。
provider    模型供应商。
base_url    模型 API base URL。
api_key     模型 API Key。
model       模型名。
secret_key  可选，部分供应商需要。
```

读取某个资产实例的 Bot 模型参数：

```http
POST /api/v1/rpc/GetBotModelConfigFFI
```

请求体：

```json
{
  "strings": ["readyclaw:[INSTANCE_ID]"],
  "ints": []
}
```

删除某个资产实例的 Bot 模型参数：

```http
POST /api/v1/rpc/DeleteBotModelConfigFFI
```

请求体：

```json
{
  "strings": ["readyclaw:[INSTANCE_ID]"],
  "ints": []
}
```

## 防护服务管理

启动防护代理：

```http
POST /api/v1/rpc/StartProtectionProxy
```

请求体：

```json
{
  "strings": [
    "{\"asset_name\":\"readyclaw\",\"asset_id\":\"readyclaw:[INSTANCE_ID]\",\"security_model\":{\"provider\":\"openai\",\"endpoint\":\"[SECURITY_MODEL_URL]\",\"api_key\":\"[API_KEY]\",\"model\":\"[MODEL_NAME]\"},\"bot_model\":{\"provider\":\"openai\",\"base_url\":\"[BOT_MODEL_URL]\",\"api_key\":\"[API_KEY]\",\"model\":\"[MODEL_NAME]\"},\"runtime\":{\"proxy_port\":0,\"audit_only\":false,\"single_session_token_limit\":0,\"daily_token_limit\":0,\"user_input_detection_enabled\":true}}"
  ],
  "ints": []
}
```

停止全部防护代理：

```http
POST /api/v1/rpc/StopProtectionProxy
```

请求体：

```json
{"strings": [], "ints": []}
```

停止指定资产实例的防护代理：

```http
POST /api/v1/rpc/StopProtectionProxyByAsset
```

请求体：

```json
{
  "strings": ["readyclaw:[INSTANCE_ID]"],
  "ints": []
}
```

查询防护代理状态：

```http
POST /api/v1/rpc/GetProtectionProxyStatus
```

请求体：

```json
{"strings": [], "ints": []}
```

查询指定资产实例的防护代理状态：

```http
POST /api/v1/rpc/GetProtectionProxyStatusByAsset
```

请求体：

```json
{
  "strings": ["readyclaw:[INSTANCE_ID]"],
  "ints": []
}
```

运行时更新防护参数：

```http
POST /api/v1/rpc/UpdateProtectionConfigByAsset
```

请求体：

```json
{
  "strings": [
    "readyclaw:[INSTANCE_ID]",
    "{\"audit_only\":true,\"single_session_token_limit\":8000,\"daily_token_limit\":100000,\"user_input_detection_enabled\":true}"
  ],
  "ints": []
}
```

## 策略接口

如果接入方更希望使用业务语义接口，而不是直接调用 RPC，可以使用防护策略接口：

```http
GET /api/v1/protection/policy
POST /api/v1/protection/policy
```

请求体示例：

```json
{
  "botId": ["readyclaw:[INSTANCE_ID]"],
  "protection": "enabled",
  "tokenLimit": {
    "session": 8000,
    "daily": 100000
  },
  "permission": {
    "open": true,
    "path": {
      "mode": "blacklist",
      "paths": []
    },
    "network": {
      "inbound": {
        "mode": "blacklist",
        "addresses": []
      },
      "outbound": {
        "mode": "blacklist",
        "addresses": []
      }
    },
    "shell": {
      "mode": "blacklist",
      "commands": []
    }
  },
  "botModel": {
    "provider": "openai",
    "id": "[MODEL_NAME]",
    "url": "[MODEL_BASE_URL]",
    "key": "[API_KEY]"
  }
}
```

`protection` 可选：

```text
enabled   启用防护。
bypass    启用但仅审计。
disabled  停用防护。
```

