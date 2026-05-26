# ClawdSecbot WebUI + OpenClaw 容器部署操作文档

## 1. 文档说明

本文档描述如何在 **1Panel OpenClaw** 应用目录中, 基于 `1panel/openclaw` 镜像挂载 **ClawdSecbot WebUI tar 包**, 同容器运行:

- **OpenClaw Gateway** — 端口 `18789`, 沿用镜像默认启动链与健康检查
- **botsec_webd** — 端口 `18080`, 提供 WebUI 版本龙虾卫士

---

## 2. 方案说明

### 2.1 架构

```text
浏览器
  ├─ :18789  → OpenClaw Gateway (镜像默认, 1Panel healthcheck)
  └─ :18080  → botsec_webd (WebUI + API 同源)

容器内路径:                          宿主机路径:
  /home/node/.openclaw              ← ./data/conf           (OpenClaw 配置)
  /home/node/.openclaw/workspace    ← ./data/workspace      (OpenClaw 工作区)
  /tmp/ClawdSecbot                  ← ./data/ClawdSecbot    (ClawdSecbot 安装路径)
  /tmp/botsec_web_workspace         ← ./data/ClawdSecbot-workspace (ClawdSecbot 数据路径)
  /tmp/.botsec                      ← ./data/ClawdSecbot-sandbox   (ClawdSecbot 防护路径)
```

### 2.2 启动链

| 组件 | 来源 | 说明 |
|------|------|------|
| PID 1 | `init: true` | Docker回收子进程 |
| 入口 | `scripts/entrypoint-botsec.sh` | 覆盖镜像默认 Cmd 不修改镜像内文件 |
| OpenClaw | entrypoint 可选预启; 沙箱启用后由 WebUI `gateway run` + `LD_PRELOAD` 直启 | 与 botsec_webd 脱钩 |
| WebUI | `/tmp/ClawdSecbot/bin/botsec_webd --addr 0.0.0.0:18080` | 容器主进程(前台 exec) |

---

## 3. 前置条件

- 存在 docker compose 环境
- 镜像例如 `1panel/openclaw:2026.5.4`
- tar包例如 `ClawdSecbot-web-1.0.5-202605271453-x86_64-community.tar.gz`
- 宿主机 CPU 架构与镜像、tar包一致
  - `x86_64` 宿主机 → `*-x86_64-*.tar.gz` + `linux/amd64` 镜像
  - `aarch64` 宿主机 → `*-arm64-*.tar.gz` + `linux/arm64` 镜像
- 应用根目录记为 **`$APP_ROOT`**(如 `/opt/1panel/apps/openclaw2`)

---

## 4. 目标结构

完成后 `$APP_ROOT` 应类似:

```text
$APP_ROOT/
├── data
│   ├── conf/                         # 不变, OpenClaw 配置
│   ├── workspace/                    # 不变, OpenClaw 工作区
│   ├── ClawdSecbot/                  # 新增, WebUI tar 包解压
│   │   ├── bin/botsec_webd
│   │   ├── web/
│   │   └── lib/libsandbox_preload.so
│   ├── ClawdSecbot-workspace/             # 持久化 → /tmp/botsec_web_workspace
│   └── ClawdSecbot-sandbox/               # 持久化 → /tmp/.botsec
├── data.yml                          # 增加 BOTSEC_WEB_PORT
├── docker-compose.yml                # 修改
└── scripts
    ├── entrypoint-botsec.sh          # 新建
    ├── init.sh
    ├── openclaw-setup-linux-amd64
    ├── openclaw-setup-linux-arm64
    └── upgrade.sh
```

WebUI tar 包解压后内层结构:

```text
ClawdSecbot-web-<version>-<build>-<arch>-<type>/
├── bin/botsec_webd
├── web/                    # 含 index.html
├── lib/libsandbox_preload.so
└── install.sh              # 宿主机安装用, 容器部署可忽略
```

---

## 5. 操作步骤

准备好 WebUI tar 包, 以下命令均在 **`cd "$APP_ROOT"`** 后执行。

### 5.1 解压 WebUI 包

```bash
mkdir -p data/ClawdSecbot
tar -xzf ClawdSecbot-web-1.0.5-202605271453-x86_64-community.tar.gz -C /tmp
cp -a /tmp/ClawdSecbot-web-*/{bin,web,lib} data/ClawdSecbot/
chmod +x data/ClawdSecbot/bin/botsec_webd
```

校验:

```bash
ls -l data/ClawdSecbot/bin/botsec_webd
ls -l data/ClawdSecbot/web/index.html
ls -l data/ClawdSecbot/lib/libsandbox_preload.so
file data/ClawdSecbot/bin/botsec_webd
```

### 5.2 创建持久化目录并设置权限

容器内进程用户为镜像中的 `node`. 将宿主机映射目录设为 **0777** 可避免 uid 不一致导致的 `mkdir permission denied`(权限不足).

```bash
mkdir -p data/ClawdSecbot-sandbox/
mkdir -p data/ClawdSecbot-workspace/
chmod -R 0777 data/ClawdSecbot-sandbox data/ClawdSecbot-workspace
```

### 5.3 创建入口脚本

创建 **`scripts/entrypoint-botsec.sh`**, 使用 **`/bin/sh`** 作为脚本解释器:

```sh
#!/bin/sh
set -eu

BOTSEC_ROOT="/tmp/ClawdSecbot"
BOTSEC_WEBD="$BOTSEC_ROOT/bin/botsec_webd"
BOTSEC_WEB_ROOT="$BOTSEC_ROOT/web"
OPENCLAW_ENTRYPOINT="/usr/local/bin/docker-entrypoint.sh"
OPENCLAW_CONF_DIR="${HOME:-/home/node}/.openclaw"

mkdir -p /tmp/botsec_web_workspace /tmp/.botsec
mkdir -p /tmp/.botsec/backups /tmp/.botsec/policies /tmp/.botsec/logs
mkdir -p /tmp/botsec_web_workspace/logs \
  /tmp/botsec_web_workspace/skills/shepherd_gate \
  /tmp/botsec_web_workspace/skills/skill_scanner
chmod -R a+rwX /tmp/.botsec /tmp/botsec_web_workspace 2>/dev/null || true

if [ ! -x "$BOTSEC_WEBD" ]; then
  echo "[entrypoint] missing botsec_webd: $BOTSEC_WEBD" >&2
  exit 1
fi
if [ ! -d "$BOTSEC_WEB_ROOT" ]; then
  echo "[entrypoint] missing web root: $BOTSEC_WEB_ROOT" >&2
  exit 1
fi

# 清理上一次运行残留的 pid / lock, 否则 `openclaw gateway run` 会拒绝启动
rm -f "$OPENCLAW_CONF_DIR/.gateway.pid" \
      "$OPENCLAW_CONF_DIR/gateway.pid" \
      "$OPENCLAW_CONF_DIR/gateway.lock" 2>/dev/null || true

# 预启 OpenClaw gateway: 用子 shell + setsid 双 fork 脱离当前会话,
# 启动后该进程的 PPID 变为 1 (docker-init/tini), 后续 WebUI 发送 SIGKILL
# 切换沙箱时, 该进程能被 PID 1 正确 reap, 不会变成 [openclaw] <defunct> 僵尸进程,
# 也避免占用 18789 端口的 socket 释放不及时。
echo "[entrypoint] pre-starting OpenClaw gateway on :18789 (detached to PID 1)"
(
  setsid "$OPENCLAW_ENTRYPOINT" node openclaw.mjs gateway --allow-unconfigured \
    </dev/null >/tmp/openclaw-bootstrap.log 2>&1 &
)

echo "[entrypoint] starting botsec_webd on :18080"
export BOTSEC_WEB_STATIC_DIR="$BOTSEC_WEB_ROOT"
exec "$BOTSEC_WEBD" --addr 0.0.0.0:18080 --web-root "$BOTSEC_WEB_ROOT"
```

**为什么用子 shell + `setsid` 双 fork**:

- `(... &)` 子 shell 启动后台命令再立刻退出, 该命令瞬间失去父进程被 PID 1 收养
- `setsid` 让 OpenClaw 进入独立 session, 与 `botsec_webd` 的会话彻底解耦
- 这样 WebUI 一键防护时, `restartOpenclawGateway` 通过 SIGKILL 杀掉旧 gateway, PID 1 的 tini 会及时 reap, 不会出现 `[openclaw] <defunct>`

**严禁** 直接写成 `"$OPENCLAW_ENTRYPOINT" ... &`, 这种写法会让 OpenClaw 的 PPID 留在 `botsec_webd`, `botsec_webd` 不会 reap 它派生时未持有的子进程, 必然产生僵尸。

**可选**: 若接受首次访问前需先点「一键防护」, 可省略 entrypoint 中的 OpenClaw 预启, 仅 `exec botsec_webd`.

```bash
chmod +x scripts/entrypoint-botsec.sh
sed -i 's/\r$//' scripts/entrypoint-botsec.sh 2>/dev/null || true
```

### 5.4 修改 docker-compose.yml

在 `openclaw2` 服务中添加 `entrypoint`、, 修改 `ports`、`volumes` :

```yaml
networks:
    1panel-network:
        external: true
services:
    openclaw2:
        container_name: ${CONTAINER_NAME}
        image: 1panel/openclaw:2026.5.4
        init: true
        restart: unless-stopped
        networks:
            - 1panel-network
        entrypoint: ["/tmp/ClawdSecbot/scripts/entrypoint-botsec.sh"]
        environment:
            HOME: /home/node
            TERM: xterm-256color
        ports:
            - ${HOST_IP}:${PANEL_APP_PORT_HTTP}:18789
            - ${HOST_IP}:${BOTSEC_WEB_PORT:-18080}:18080
        volumes:
            - ./data/conf:/home/node/.openclaw
            - ./data/workspace:/home/node/.openclaw/workspace
            - ./data/ClawdSecbot:/tmp/ClawdSecbot
            - ./scripts/entrypoint-botsec.sh:/tmp/ClawdSecbot/scripts/entrypoint-botsec.sh
            - ./data/ClawdSecbot/lib/libsandbox_preload.so:/usr/lib/clawdsecbot/libsandbox_preload.so
            - ./data/ClawdSecbot-workspace:/tmp/botsec_web_workspace
            - ./data/ClawdSecbot-sandbox:/tmp/.botsec
            - /etc/localtime:/etc/localtime
        deploy:
            resources:
                limits:
                    cpus: ${CPUS}
                    memory: ${MEMORY_LIMIT}
        healthcheck:
            interval: 3m
            retries: 3
            start_period: 15s
            timeout: 10s
            test:
                - CMD
                - node
                - -e
                - fetch('http://127.0.0.1:18789/healthz').then((r)=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))
        labels:
            createdBy: Apps
```

### 5.5 修改 data.yml

在 `additionalProperties` 的formFields数组中 (文件末尾) 添加:

```yaml
    - default: 18080
      edit: true
      envKey: BOTSEC_WEB_PORT
      labelEn: ClawdSecbot WebUI Port
      labelZh: ClawdSecbot WebUI 端口
      required: true
      rule: paramPort
      type: number
      label:
        en: ClawdSecbot WebUI Port
        es-es: Puerto ClawdSecbot WebUI
        ja: ClawdSecbot WebUI ポート
        ms: Port ClawdSecbot WebUI
        pt-br: Porta ClawdSecbot WebUI
        ru: Порт ClawdSecbot WebUI
        ko: ClawdSecbot WebUI 포트
        zh-Hant: ClawdSecbot WebUI 埠
        zh: ClawdSecbot WebUI 端口
        tr: ClawdSecbot WebUI Portu
      description:
        en: "ClawdSecbot security console. Access http://IP:Port after deploy. Default 18080."
        ja: "ClawdSecbot セキュリティコンソール。デプロイ後 http://IP:Port でアクセス。デフォルト 18080。"
        ms: "Konsol keselamatan ClawdSecbot. Akses http://IP:Port selepas deploy. Lalai 18080."
        pt-br: "Console de segurança ClawdSecbot. Acesse http://IP:Port após deploy. Padrão 18080."
        ru: "Консоль безопасности ClawdSecbot. После развертывания: http://IP:Port. По умолчанию 18080."
        ko: "ClawdSecbot 보안 콘솔. 배포 후 http://IP:Port 로 접속. 기본 18080."
        tr: "ClawdSecbot guvenlik konsolu. Deploy sonrasi http://IP:Port. Varsayilan 18080."
        zh-Hant: "ClawdSecbot 安全控制台。部署後使用 http://IP:埠 訪問，預設 18080。"
        zh: "ClawdSecbot 安全防护 Web 控制台。部署后通过 http://IP:端口 访问，默认 18080。"
        es-es: "Consola de seguridad ClawdSecbot. Tras el despliegue: http://IP:Puerto. Predeterminado 18080."
```

### 5.6 启动与验证

```bash
docker compose config
docker compose up -d
docker compose logs --tail=100
```

预期日志:

```text
[entrypoint] starting OpenClaw gateway on :18789
[entrypoint] starting botsec_webd on :18080
[webbridge] listening on http://0.0.0.0:18080
```

**进程与健康检查:**

```bash
docker exec "${CONTAINER_NAME}" ps aux
curl -fsS "http://127.0.0.1:${PANEL_APP_PORT_HTTP:-18789}/healthz"
curl -fsS "http://127.0.0.1:${BOTSEC_WEB_PORT:-18080}/health"
```

**防护功能使用:**

1.访问 http://IP:18080/health 获取管理员账号密码并记住
2.开始安全扫描, 检测到openclaw, 配置安全模型+Bot模型并一键防护
3.访问 http://IP:18789/ 跟openclaw对话

---

## 6. 升级方案

### 6.1 升级 WebUI 包

```bash
cd "$APP_ROOT"
cp -a data/ClawdSecbot "data/ClawdSecbot.bak.$(date +%Y%m%d%H%M)"

# 解压新版本覆盖
tar -xzf ClawdSecbot-web-<新版本>.tar.gz -C /tmp
cp -a /tmp/ClawdSecbot-web-*/{bin,web,lib} data/ClawdSecbot/
chmod +x data/ClawdSecbot/bin/botsec_webd

docker compose restart
```

`data/ClawdSecbot-workspace` 与 `data/ClawdSecbot-sandbox` **通常保留**, 无需删除。

### 6.2 升级 OpenClaw 镜像

1. 修改 `docker-compose.yml` 中 `image` 标签
2. 执行 `docker compose pull && docker compose up -d`
3. 确认 `entrypoint`、volumes 未被 1Panel 模板覆盖
4. 验证 `18789/healthz` 与 `18080/health`

### 6.3 回滚

```bash
rm -rf data/ClawdSecbot
mv data/ClawdSecbot.bak.XXXXXXXX data/ClawdSecbot
docker compose up -d
```

临时停用 WebUI: 移除 compose 中 `entrypoint`、Web 相关 volumes 及 `18080` 端口映射, 恢复镜像默认启动。
