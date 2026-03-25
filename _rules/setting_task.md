// 策略类数据 来自资产的防护设置 跟外部进程通信
{
  "botId": "xxxxxxxxxxxxxxxxxx",
  "protection": "enabled|bypass|disabled",
  "userRules": [
    "不允许删除任何文件"
  ],
  "tokenLimit": [
    "session": 111,
    "daily": 222
  ],
  "permission": [
    "open": true,
    "path": {
      "mode": "blacklist",
      "paths": ["/etc", "/var/root"]
    },
    "network": {
      "inbound":  { "mode": "blacklist", "addresses": [] },
      "outbound": { "mode": "blacklist", "addresses": [] }
    },
    "shell": {
      "mode": "blacklist",
      "commands": ["rm -rf"]
    }
  ],
  "botModel": {
    "provider": "openai",
    "id": "glm-4.5",
    "url": "xxx",
    "key": "xxx"
  }
}