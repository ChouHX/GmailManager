# Gmail Manager

Gmail 账号管理与 IMAP 取件控制台。**React + shadcn/ui + KokonutUI + Tailwind CSS v4 + Go + SQLite**，前后端打包在同一个容器内，静态资源直接嵌入 Go 二进制。

```
┌───────────────────────────────────────────────┐
│ 单一容器                                       │
│  Go (net/http)                                │
│   ├── /api/*   JSON API + 会话 Cookie          │
│   └── /*       embed 的 React SPA              │
│  SQLite  /data/gmailmanager.db                │
│  密钥    /data/secret.key（或 GM_SECRET）      │
└───────────────────────────────────────────────┘
```

## 前端栈

- **shadcn/ui**：Button / Input / Dialog / Select / Table / Tabs / Checkbox / Switch / Badge / Tooltip 等基础组件，源码在 `web/src/components/ui`，可自由修改
- **KokonutUI**（[kokonutui.com](https://kokonutui.com)）：`profile-dropdown`（顶栏账号菜单，含主题切换与按住退出）、`status-island`（批量取件进度浮岛）、`split-flap-text`（登录页翻牌副标题）、`loader`（加载态）
- **Tailwind CSS v4** + CSS 变量主题：`web/src/index.css` 定义 `:root` / `.dark` 两套 token，全站支持深色模式
- 无侧边栏：三个导航项直接放在顶栏，节省横向空间
- Dialog 遮罩只用半透明纯色，**不使用 `backdrop-filter`**，避免无 GPU 加速的机器出现卡顿

## 功能

### 1. 账号管理（表格）
- 导入 / 导出，格式：`邮箱----密码----2FA密钥----应用专用密码`
  - 兼容 `----`、Tab、`|`、`,` 分隔；忽略空行与 `#` 注释行
  - 导入支持「覆盖已存在账号」，逐行返回新增 / 更新 / 跳过 / 失败明细
- 增删改查、按邮箱或备注搜索、分页（默认每页 10 条）、批量选择与批量删除
- 导出全部或仅导出选中项
- 内置 TOTP（RFC 6238）实时计算 2FA 验证码，30 秒自动刷新
- 一键「验证 IMAP 登录」，直接确认应用专用密码是否可用
- 记录每个账号的最近取件时间、状态、最新邮件主题

### 2. 账号取件（表格）
- 顶部 **批量取件** 按钮：多线程（前端 5 并发 + 服务端并发上限）获取当前页每个账号的最新 1 封邮件
- 勾选账号后，批量取件只作用于所选账号
- 每行最后一列 **取件** 按钮：获取该账号最新 10 封邮件，在 Dialog 中展示列表
- 点击列表中的邮件 → 查看正文详情（HTML 安全净化渲染 / 纯文本切换 / 附件列表 / 复制全文）
- 取件范围默认 **收件箱 + 垃圾箱（含已删除）**，可在页面顶部切换为仅收件箱 / 仅垃圾箱 / 仅已删除
- 批量取件时底部 KokonutUI **StatusIsland** 实时显示每个账号的结果，悬停/点击可展开完整列表
- 失败原因直接显示在表格里，悬停可看完整错误

## 快速开始（Docker）

镜像由 GitHub Actions 在每次推送到 `main` 时构建，并推送到 GitHub Container Registry。本地**只需拉取，无需构建**：

```bash
# 1. 拉取最新镜像并启动
docker compose pull
docker compose up -d

# 2. 打开 http://localhost:8080
#    默认账号 admin / admin123 —— 请务必修改！
```

镜像地址：`ghcr.io/chouhx/gmailmanager:latest`

### 镜像标签

| 触发方式 | 标签 |
| --- | --- |
| 推送到 `main` | `latest`、`main`、`sha-<短哈希>` |
| 推送 `v1.2.3` 标签 | `1.2.3`、`1.2`、`latest` |
| Pull Request | 仅构建验证，不推送 |

> GHCR 上的包默认是私有的。首次使用前，到 GitHub → 你的头像 → Packages → `gmailmanager` → Package settings，把可见性改为 Public；或先 `docker login ghcr.io -u <用户名>` 再拉取。

### 从源码构建（可选）

不想用 CI 产物时，用覆盖文件从本地源码构建：

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```

国内网络较慢时，修改 `docker-compose.build.yml` 里的 `build.args`：

```yaml
args:
  NPM_REGISTRY: https://registry.npmmirror.com
  GOPROXY: https://goproxy.cn,direct
```

生产部署请至少设置：

```yaml
environment:
  GM_ADMIN_USER: admin
  GM_ADMIN_PASSWORD: "换成强密码"
  GM_SECRET: "换成一串足够长的随机字符串"   # 不设置则自动生成到 /data/secret.key
```

> 修改 `GM_SECRET` 后，之前用旧密钥加密的数据将无法解密，请勿随意变更；若不设置，密钥随 `/data` 卷一起持久化。

### 版本信息

`GET /api/health` 会返回构建版本与提交号，便于确认线上跑的是哪个镜像：

```bash
curl -s localhost:8080/api/health
# {"status":"ok","time":...,"version":"main","commit":"a2f95fb..."}
```

### 数据持久化

数据库与密钥都在 `/data`（compose 中映射为命名卷 `gmailmanager-data`）。
备份时同时保留 `gmailmanager.db` 与 `secret.key`（或记住 `GM_SECRET`），否则密文无法还原。

## 本地开发

```bash
# 后端（默认 :8080，数据写入 ./data）
go run .

# 前端（:5173，/api 代理到 :8080）
cd web && npm install && npm run dev
```

构建生产前端后，`go build` 会把 `web/dist` 嵌入二进制：

```bash
cd web && npm run build && cd .. && go build -o gmailmanager .
```

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `GM_ADMIN_USER` | `admin` | 管理员用户名 |
| `GM_ADMIN_PASSWORD` | `admin123` | 管理员密码 |
| `GM_SECRET` | 自动生成 | 凭据加密密钥（AES-256-GCM） |
| `GM_DATA_DIR` | `/data` | 数据目录 |
| `GM_DB_PATH` | `/data/gmailmanager.db` | SQLite 路径 |
| `GM_ADDR` | `:8080` | 监听地址 |
| `GM_IMAP_HOST` | `imap.gmail.com` | IMAP 服务器 |
| `GM_IMAP_PORT` | `993` | IMAP TLS 端口 |
| `GM_FETCH_CONCURRENCY` | `6` | 服务端并发取件上限 |
| `GM_FETCH_TIMEOUT_SECONDS` | `60` | 单账号取件超时 |
| `GM_FETCH_LIMIT` | `10` | 单账号默认取件数量 |
| `GM_IMAP_PLAINTEXT` | `false` | 仅测试用：关闭 IMAP TLS（Gmail 必须为 false） |
| `GM_SESSION_HOURS` | `12` | 会话有效期 |
| `GM_COOKIE_SECURE` | `false` | 启用 HTTPS 后设为 `true` |

## 技术要点

- **取件**：`github.com/emersion/go-imap` 直连 `imap.gmail.com:993`（TLS），登录后按 `\Junk` / `\Trash` 特殊用途标记自动定位垃圾箱与已删除目录，三个目录合并后按时间倒序截取。
- **邮件正文**：`go-message` 解析 MIME（自动处理非 UTF-8 字符集），HTML 经 `bluemonday` 白名单净化后才交给前端渲染，移除脚本与事件属性。
- **凭据安全**：密码、2FA 密钥、应用专用密码均以 AES-256-GCM 加密入库；列表接口只返回掩码，绝不回传明文。
- **会话**：HMAC-SHA256 签名的 HttpOnly Cookie，登录失败 5 次锁定 5 分钟。
- **并发**：单账号取件受全局信号量限制，避免触发 Gmail 的并发连接限制。

## 目录结构

```
main.go                     入口、静态资源嵌入与 SPA 回退
internal/config             环境变量配置
internal/store              SQLite 数据访问、导入导出格式
internal/secret             AES-256-GCM 加密
internal/totp               TOTP 计算
internal/imapx              IMAP 取件与 MIME 解析
internal/api                HTTP API、会话、中间件
web/                        React + shadcn/ui + KokonutUI 前端（Vite）
Dockerfile                  三阶段构建（前端 → Go → alpine 运行时）
docker-compose.yml          一键部署
```

## 测试

```bash
go test ./...
```

覆盖：导入导出格式解析、账号 CRUD 与分页、加密往返与错误密钥、TOTP 标准测试向量、IMAP 取件（内置内存 IMAP 服务器，验证收件箱/垃圾箱/已删除合并、数量限制、错误密码、邮件详情与 HTML 净化）、HTTP API 全流程（登录鉴权、CRUD、导入、批量取件参数校验、SPA 回退）。
