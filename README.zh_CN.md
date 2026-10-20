<div align="center">

![new-mcp](/web/public/logo.png)

# New MCP

🍥 **下一代统一 MCP 服务管理平台**

<p align="center">
  <strong>简体中文</strong> |
  <a href="./README.md">English</a>
</p>

<p align="center">
  <a href="https://mcp.newmcp.pro/">
    <img src="https://img.shields.io/badge/在线体验-mcp.newmcp.pro-brightgreen" alt="在线体验">
  </a><!--
  --><a href="https://raw.githubusercontent.com/mujiajun-index/new-mcp/main/LICENSE">
    <img src="https://img.shields.io/github/license/mujiajun-index/new-mcp?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://github.com/mujiajun-index/new-mcp/releases/latest">
    <img src="https://img.shields.io/github/v/release/mujiajun-index/new-mcp?color=brightgreen&include_prereleases" alt="release">
  </a><!--
  --><a href="https://hub.docker.com/r/mujkjk/new-mcp">
    <img src="https://img.shields.io/badge/docker-dockerHub-blue" alt="docker">
  </a>
</p>

<p align="center">
  <a href="#-快速开始">快速开始</a> •
  <a href="#-核心功能">核心功能</a> •
  <a href="#-系统架构">系统架构</a> •
  <a href="#-部署">部署</a>
</p>

</div>

---

## 📝 项目简介

NewMCP 是一个统一的 MCP（Model Context Protocol）服务管理平台。提供服务注册、分组、路由和协议桥接功能——让你在一个平台上管理所有 MCP 服务，并通过统一的网关端点对外暴露。

> [!NOTE]
> 本项目仅供个人学习与研究使用，面向希望自建和管理 MCP 服务的开发者。

---

## ✨ 核心功能

### 🎯 基础功能

| 功能 | 说明 |
|------|------|
| 📋 服务注册 | 注册和测试 MCP 服务，发现工具、资源、资源模板及提示词 |
| 🗂️ 分组管理 | 独立端点、Direct/Smart 模式，以及分组内工具/资源/提示词的启用控制 |
| 🌐 MCP 网关 | 通过 HTTP 或 WebSocket 端点聚合各 API Key 授权范围内的服务 |
| 🔀 协议桥接 | 在 stdio、SSE、HTTP、WebSocket 和被动连接之间桥接 |
| 🔑 双重认证 | JWT 用户认证 + API Key 用于 MCP 客户端访问 |
| 🛡️ 角色权限 | 超级管理员、管理员、普通用户角色及用户分组控制 |
| 🔐 上游密钥池 | SSE 和 Streamable HTTP 支持单/多密钥、随机/轮询、请求头/查询参数认证及 401/403 自动禁用 |
| 📊 运行维护 | 调用日志、健康汇总、stdio 进程资源用量及启动/停止/重启 |

### 🤖 传输协议支持

- **stdio** — 仅管理员可注册和测试；在 NewMCP 主机或容器中通过 `npx`、`uvx` 或自定义命令运行 MCP 进程
- **SSE** — 服务端推送事件
- **Streamable HTTP** — 支持 MCP `2026-07-28` 请求与 POST 订阅流，保留至 `2025-11-25` 的旧版兼容
- **WebSocket** — 支持客户端网关连接，以及主动连接上游 `ws://` / `wss://` MCP 服务
- **被动 WebSocket 接入** — 注册服务后生成接入地址，本地 MCP 服务通过 WebSocket 桥接程序主动连接并提供工具

被动接入地址根据 `ServerAddress` 生成 WS 或 WSS 链接，使用独立接入凭证。每个端点对应一个 MCP 服务，最近成功初始化的连接替换旧连接。服务页面提供接入地址、连接状态、工具同步及凭证重置。连接与代理配置见 [API 文档](./docs/API.md) 和 [部署文档](./docs/DEPLOY.md)。

### 🧠 智能发现

- **直连模式** — 暴露所选分组中已授权的工具
- **智能模式** — 通过 BM25 渐进式发现目录；管理员可为指定用户分组启用 Jev/CLM/Laya 驱动的 `mcp.smart_search` 语义搜索
- **工具命名空间** — 自动命名隔离（`{服务名}__{工具名}`），避免冲突

### ☁️ 设备集成

- 主动连接云平台（小智等）
- WebSocket 长连接设备控制
- 摄像头与视觉模型集成支持

### 🎨 内置 AI 服务

创建配置、测试并启用虚拟 MCP 服务，再将服务加入 API Key 可访问的分组即可调用。

| 服务 | 能力 |
|------|------|
| 视觉理解 | 使用 `analyze_image` 分析图片，配合 `upload_image` 上传本地参考图 |
| 图片生成 | OpenAI 兼容、Gemini、OpenRouter 协议；支持文生图、图片编辑及持久化后台任务，通过 `get_image_status` 查询 |
| 视频生成 | OpenAI 兼容、Gemini、OpenRouter 协议；支持文生视频、单图图生视频、持久化任务、`get_video_status` 查询、播放、下载及重试保存 |
| 摄像头 | 抓拍画面、分析画面和实时推流 |
| System One | 通过 `evaluate` 接入 Jev/CLM/Laya 决策模型，支持概率、选项、评分及批量评估 |

图片和视频生成在短暂等待后返回本地 `task_id`，使用同配置的状态工具继续查询。每种服务分别配置上游凭证及模型能力，详见[图片生成](./docs/IMAGE-GENERATION.md)、[视频生成](./docs/VIDEO-GENERATION.md)和[决策模型](./docs/SYSTEM-ONE.md)文档。

### 🛒 服务市场、计费与存储

- **服务市场** — 管理员维护条目、分类和标签；支持引用式安装、stdio 共享/独占进程，以及工具/资源/提示词的条目级定价。
- **可选计费** — 支持市场服务按次计费、钱包、额度、兑换码、邀请奖励、用户分组倍率及计费日志。计费默认关闭；自有服务在 NewMCP 内免费，上游服务商可能另行收费。
- **媒体存储** — 支持本地磁盘和 S3 兼容存储、图片签名链接、直传、用户配额、保留规则和自动清理。图片/视频保留时间可设为 `0`，表示永久保存。

---

## 🏗 系统架构

```
┌─────────────┐     ┌─────────────────────────────┐     ┌──────────────┐
│  MCP 客户端  │────▶│       NewMCP 网关            │────▶│  MCP 服务    │
│ (Claude等)  │◀────│  /mcp  /mcp/group/{slug}    │◀────│ (stdio/SSE/  │
└─────────────┘     │        /smart/mcp            │     │  HTTP/WS)    │
                    └─────────────────────────────┘     └──────────────┘
                           │
                    ┌──────┴──────┐
                    │   后端服务   │
                    │  Go + Gin   │
                    │  GORM + DB  │
                    └──────┬──────┘
                           │
                    ┌──────┴──────┐
                    │   前端界面   │
                    │ React + TS  │
                    └─────────────┘
```

### MCP 网关

NewMCP 通过统一网关暴露 MCP 工具，支持两种**工具暴露模式**，由端点路由驱动：

| 端点 | 传输 | 模式 | 说明 |
|------|------|------|------|
| `POST /mcp` | Streamable HTTP | 固定 Direct | 聚合 API Key 绑定的所有分组，去重后暴露全部工具（`serviceName__toolName`） |
| `POST /smart/mcp` | Streamable HTTP | 固定 Smart | 聚合授权分组，暴露 5 个核心元工具及可选的 `mcp.smart_search` |
| `POST /mcp/group/{slug}` | Streamable HTTP | 由分组 `expose_mode` 决定 | 端点驱动，每个分组独立配置 |

客户端 WebSocket 网关路由（`/mcp/ws`、`/smart/mcp/ws`、`/mcp/ws/group/{slug}`）通过 MCP JSON-RPC 消息传递请求、响应及订阅通知。上游 WebSocket 服务注册与 `/mcp/passive/` 被动接入也可用。

网关实现 MCP `2026-07-28`：`server/discover`、每次请求的 `_meta`、HTTP 路由镜像头、`resultType` 和 `subscriptions/listen`。旧客户端可继续通过 `initialize` 和携带 `Mcp-Session-Id` 的 GET SSE 使用旧版协议。MCP 用日期标识版本，JSON-RPC `2.0` 与 SDK 版本是不同概念。NewMCP 的 WebSocket 路由传递原始 MCP JSON-RPC 帧，需要兼容的客户端或桥接程序。

实验 Events API 提供 `events/list`、`events/poll`、`events/stream`、`events/subscribe`、`events/unsubscribe`，内建四种网关变更事件：`mcp.tools.list_changed`、`mcp.resources.list_changed`、`mcp.prompts.list_changed`、`mcp.resources.updated`。在 API Key 授权范围内支持轮询、推送及签名 HTTPS webhook。Webhook 订阅在内存中最多保留五分钟，需定期续期，重启后客户端须重新订阅。当前范围为网关目录及资源变更，上游业务事件需另行接入。Smart 模式的标准订阅只确认实际支持的过滤项，实验事件按授权服务范围订阅。详见[协议与 curl 示例](./docs/MCP-PROTOCOL.md#11-订阅与实验-events)和 [API 文档](./docs/API.md#11-mcp-协议端点)。

- **Direct（直连）模式** — 直接暴露所有工具，适合支持大量工具的 LLM 客户端（Claude Code、Cursor 等）。
- **Smart（智能）模式** — 暴露 5 个核心元工具（`mcp.search` / `mcp.describe` / `mcp.execute` / `mcp.execute_batch` / `mcp.read`），客户端通过 搜索 → 查看 → 执行 渐进发现和调用工具；相互独立的调用可用 `mcp.execute_batch` 一次并发执行。管理员为调用者所属用户分组启用语义搜索后，会额外暴露 `mcp.smart_search`。适合上下文受限的设备（如小智）或工具量特别大的场景。
- 分组端点（`/mcp/group/{slug}`）的模式由每个分组的 `expose_mode` 设置（`direct` / `smart`）决定。

### 技术栈

| 层级 | 技术 |
|------|------|
| **后端** | Go 1.26.2+, Gin, GORM v2 |
| **前端** | React 19, TypeScript, Rsbuild, Radix UI, Tailwind CSS 4 |
| **数据库** | SQLite（默认）, MySQL, PostgreSQL |
| **缓存** | 内存缓存与会话池 |
| **存储** | 本地磁盘、S3 兼容对象存储 |
| **认证** | JWT, API Key |
| **协议** | MCP (Model Context Protocol) |

### 项目结构

```
├── cmd/server/        — 应用入口
├── router/            — HTTP 路由
├── controller/        — 请求处理
├── service/           — 业务逻辑
├── model/             — 数据模型 (GORM)
├── middleware/        — 认证、限流、CORS
├── billing/           — 市场定价与额度结算
├── internal/          — 内部包（MCP 适配器、网关）
├── common/            — 公共工具
├── dto/               — 数据传输对象
├── frontend.go        — 嵌入已生成的 web/dist 产物
├── web/               — React 前端
├── scripts/           — 数据迁移与验证工具
└── docs/              — 项目文档
```

---

## 🚀 快速开始

### 使用 Docker Compose 启动

需要安装 Docker 和 Compose 插件。默认使用已发布镜像和 SQLite，无需在本机安装 Go 或 Node.js。

```bash
# 克隆项目
git clone https://github.com/mujiajun-index/new-mcp.git
cd new-mcp

# 启动前编辑 docker-compose.yaml，将 SESSION_SECRET 和 CRYPTO_SECRET
# 分别替换为至少 32 个字符的独立随机字符串。
docker compose up -d
```

打开 `http://localhost:3000`。首次访问会进入 `/setup`，创建超级管理员账号（密码至少 8 个字符），项目不提供默认管理员账号和密码。数据和日志分别保存在 `./data`、`./logs`。

### 接入第一个 MCP 客户端

1. 登录后，在**系统设置 → 服务器地址**（`ServerAddress`）填写客户端可访问的地址；远程访问时使用对外的 HTTPS 地址。
2. 注册并测试一个 MCP 服务、安装市场条目，或启用内置 AI 配置。
3. 创建分组、加入服务，并启用需要暴露的工具/资源/提示词。
4. 创建 API Key，授权该分组或自己的全部分组。未授权任何分组的 API Key 不会暴露服务。
5. 从分组页面复制连接配置到 MCP 客户端，并替换占位密钥。例如，支持 `mcpServers` 和 Streamable HTTP 的客户端可参考：

```json
{
  "mcpServers": {
    "newmcp": {
      "type": "streamable-http",
      "url": "http://localhost:3000/mcp/group/my-group",
      "headers": {
        "X-API-Key": "sk-your-api-key"
      }
    }
  }
}
```

将 `my-group` 替换为实际分组标识。使用 `/mcp` 可聚合该密钥授权的全部分组，使用 `/smart/mcp` 可启用智能发现。网关支持 `X-API-Key` 或 `Authorization: Bearer sk-...`；网页登录 JWT 用于管理 API。不同客户端的配置字段可能不同，请按客户端要求调整。

### 从源码运行

需要 **Go 1.26.2+**、**Node.js 22.13+（22.x）或 24+**、npm，Make 可选。这里的 Node.js 版本同时满足锁文件中的前端构建和 lint 依赖。运行 stdio 服务还需在后端主机安装对应命令/运行时；Docker 镜像已包含 Node.js/npm 和 Python，并可在需要时安装 `uv`。

在克隆后的项目目录执行：

```bash
cp .env.example .env
# 编辑 .env，分别设置独立的 SESSION_SECRET 和 CRYPTO_SECRET。

# 必须先构建前端，再编译后端。
cd web
npm ci
npm run build
cd ..

make run
# 不使用 Make：go run ./cmd/server/
```

后端从工作目录加载 `.env`，默认在 `http://localhost:3000` 提供嵌入的前端页面。`frontend.go` 使用 `go:embed`，因此全新克隆后执行 Go 构建、运行或测试前，都需要先生成 `web/dist`。使用嵌入页面时，前端改动后需重新打包前端、重新编译并启动后端。

Windows PowerShell 可用 `Copy-Item .env.example .env`；未安装 Make 时直接执行 `go run ./cmd/server/`。

### 前端开发

```bash
cd web
npm ci
npm run dev
```

保持 Go 后端运行在 `3000` 端口，浏览器访问 `http://localhost:5173`。Rsbuild 将 `/api/`、`/v/`、`/mcp/` 代理至后端；MCP 客户端应连接 `3000` 端口或实际部署的网关地址。

---

## 🚢 部署

### 构建

```bash
# 先按上文生成 web/dist，再编译二进制文件。
make build

# 输出文件位于 build/newmcp
```

### 运行

```bash
./build/newmcp
```

从包含 `.env` 和持久化数据的目录启动。不使用 Make 时执行 `go build -o build/newmcp ./cmd/server/`；Windows 使用 `-o build/newmcp.exe` 构建，再执行 `.\build\newmcp.exe`。程序启动时自动迁移数据库表结构，升级前应备份数据库、存储文件和密钥。

### Docker

使用 Docker 拉取并运行（前端和后端均运行在 3000 端口）：

```bash
# 拉取镜像
docker pull mujkjk/new-mcp:latest

# 先将 .env.example 复制为 .env，并设置两个密钥。
docker run --name new-mcp -d --restart always \
  -p 3000:3000 \
  --env-file .env \
  -e GIN_MODE=release \
  -e DB_PATH=/app/data/newmcp.db \
  -e TZ=Asia/Shanghai \
  -v "$(pwd)/data:/app/data" \
  -v "$(pwd)/logs:/app/logs" \
  mujkjk/new-mcp:latest --log-dir /app/logs
```

以上命令使用 Bash 语法，跨平台启动可使用 Compose。若要从本地源码构建完整镜像，执行 `docker build -t new-mcp:local .`。

#### 环境变量

| 变量 | 默认值（源码 / Docker 镜像） | 说明 |
|------|-----------------------------|------|
| `PORT` | `3000` | 服务端口 |
| `GIN_MODE` | `debug` / `release` | Gin 模式（`debug`/`release`） |
| `DB_TYPE` | `sqlite` | 数据库类型（`sqlite`/`mysql`/`postgres`） |
| `DB_PATH` | `./data/newmcp.db` / `/app/data/newmcp.db` | SQLite 数据库路径 |
| `SQL_DSN` | — | MySQL/PostgreSQL 连接字符串 |
| `SESSION_SECRET` | 未设置 | JWT 和签名链接密钥，首次使用前设置高强度且固定的值 |
| `CRYPTO_SECRET` | 未设置 | 敏感数据加密密钥，需保持稳定以解密已保存的凭证 |
| `LOG_LEVEL` | `info` | 日志级别：`debug` / `info` / `warn` / `error` |
| `TZ` | 主机/容器时区 | 进程时区，示例使用 `Asia/Shanghai` |
| `REDIS_CONN_STRING` | 未设置 | 预留配置，目前仅被读取，尚未接入 Redis 客户端或缓存 |

文件日志默认写入 `./logs`。可使用命令行参数 `--log-dir /path/to/logs` 修改目录，或通过 `--log-dir ""` 仅保留控制台日志。两个密钥都应设置非空值：未配置 `SESSION_SECRET` 时签名使用内置回退值；未配置 `CRYPTO_SECRET` 时敏感数据仅作 base64 编码，并未加密。

> [!NOTE]
> MCP 端点地址由**系统设置**中的**服务器地址**（`ServerAddress`）决定，管理员可在后台运行时动态修改，无需重启或配置环境变量。

存储、保留时长、SMTP、注册、计费、限流和语义搜索等选项也通过管理后台和数据库维护，并非任意同名环境变量均可配置。使用本地媒体存储时，服务器地址需能被客户端以及需要拉取参考图的上游服务访问。修改存储后端或连接配置后需重启 NewMCP 生效。

#### 使用 MySQL / PostgreSQL

在本机 `.env`、Docker 的 `--env-file` 文件或 Compose 的 `environment` 中设置 `DB_TYPE` 和 `SQL_DSN`，连接已创建的数据库和账号：

```dotenv
# MySQL
DB_TYPE=mysql
SQL_DSN=newmcp:password@tcp(mysql-host:3306)/newmcp?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai

# PostgreSQL（改用以下值，不要同时启用 MySQL 配置）
# DB_TYPE=postgres
# SQL_DSN=host=postgres-host port=5432 user=newmcp password=password dbname=newmcp sslmode=disable
```

PostgreSQL 的 TLS 选项应按数据库实际要求设置。Docker 中使用的数据库主机名必须能从应用容器访问。切换数据库类型不会自动迁移已有数据；SQLite 转 MySQL 可参考 [scripts/sqlite_to_mysql.py](./scripts/sqlite_to_mysql.py)。

#### Docker Compose

> [!TIP]
> 仓库中的 [docker-compose.yaml](./docker-compose.yaml) 默认使用 `image: mujkjk/new-mcp:latest`。启动前替换其中的两个密钥占位值；该文件不会自动将项目 `.env` 中的变量传入容器。

```bash
# 拉取镜像并在后台启动
docker compose pull
docker compose up -d

# 查看日志
docker compose logs -f new-mcp

# 停止
docker compose down

# 更新到最新发布镜像
docker compose pull
docker compose up -d
```

默认使用 **`./data:/app/data`** 和 **`./logs:/app/logs`** 绑定挂载，SQLite 和本地媒体文件持久化在 `./data` 中；执行 `docker compose down` 停止和移除容器后，这些主机目录仍会保留。

需要从源码构建时，取消 `build: .` 的注释，并将服务镜像改为 `new-mcp:local` 等本地标签，再执行 `docker compose up -d --build`。默认仅使用镜像的配置不会重新构建本地代码。

文件内包含可选的 MySQL/PostgreSQL 服务模板。启用对应数据库服务、命名卷、`DB_TYPE`、`SQL_DSN` 及依赖/健康检查，并确保 DSN 凭证与数据库账号配置一致。当前实现无需 Redis。

使用反向代理时，启用 WebSocket 升级、关闭 SSE 缓冲，并允许长连接请求；代理完整应用，包括 `/mcp`、`/smart/mcp`、`/mcp/passive/`、`/api/`、`/u/` 和 `/v/`。将 `ServerAddress` 设为应用对外地址；其他来源的浏览器 MCP 集成需配置 `MCPAllowedOrigins`。运行后可检查：

```bash
curl http://localhost:3000/health
# {"status":"ok","version":"..."}
```

### Make 命令一览

| 命令 | 说明 |
|------|------|
| `make build` | 编译 Go 二进制文件，需先生成 `web/dist` |
| `make run` | 编译并运行后端，页面来自嵌入的前端产物 |
| `make dev` | 通过 `go run` 运行后端，不含自动重载或前端构建 |
| `make test` | 运行 Go 测试（`go test ./... -v`），需先生成 `web/dist` |
| `make tidy` | 整理 Go 模块 |
| `make clean` | 删除 **`build/` 和 `data/`**，包含本地数据库/媒体文件，执行前先备份 |

前端检查（在 `web/` 内执行）：`npm run typecheck`、`npm run lint`、`npm run build:check`。前端工具函数专项测试：`npm run test:videogen-params`、`npm run test:task-history`、`npm run test:datetime`。

---

## 📚 文档

| 文档 | 说明 |
|------|------|
| [产品需求文档](docs/PRD.md) | PRD 产品需求 |
| [系统架构](docs/ARCHITECTURE.md) | 架构设计 |
| [API 文档](docs/API.md) | 接口文档 |
| [数据库设计](docs/DATABASE.md) | 数据库表结构 |
| [MCP 协议](docs/MCP-PROTOCOL.md) | MCP 协议详情 |
| [部署指南](docs/DEPLOY.md) | 部署说明 |
| [前端设计](docs/FRONTEND.md) | 前端架构 |
| [小智集成](docs/XIAOZHI-INTEGRATION.md) | 小智设备对接 |
| [图片生成](docs/IMAGE-GENERATION.md) | 上游协议、编辑、异步任务与结果存储 |
| [视频生成](docs/VIDEO-GENERATION.md) | 上游协议、文生/图生视频、任务跟踪与下载 |
| [System One](docs/SYSTEM-ONE.md) | 决策模型配置与语义搜索 |
| [媒体上传与存储](docs/VISION-IMAGE-TRANSPORT.md) | 图片上传、签名链接、S3、保留规则与配额 |
| [摄像头 Agent 调用](docs/CAMERA-AGENT-CALLING.md) | 摄像头抓拍与分析流程 |
| [上游多密钥](docs/MULTI-KEY.md) | 密钥池、选择策略与认证方式 |
| [计费设计](docs/COMMERCIALIZATION.md) | 市场定价、钱包、额度与结算 |

详细文档包含历史规划和设计草案；如有差异，请结合本 README 的命令与默认值，以及仓库当前配置和源码使用。

---

## 🌟 Star 历史

<div align="center">

[![Star History Chart](https://api.star-history.com/chart?repos=mujiajun-index/new-mcp&type=date&legend=top-left)](https://www.star-history.com/?type=date&repos=mujiajun-index%2Fnew-mcp)

</div>

---

## 📜 许可证

除文件另有声明外，NewMCP 基于 [GNU Affero General Public License 第 3 版或任意后续版本](./LICENSE)（`AGPL-3.0-or-later`）授权。项目版权和授权声明见 [NOTICE](./NOTICE)；第三方组件保留各自许可，包括 [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md) 中列出的 MIT 许可代码。

协议允许商业使用。分发源码、二进制或容器镜像时，应履行适用的许可声明和对应源码提供义务。如果修改 NewMCP 并让用户通过网络与该版本交互，应按 [AGPLv3 第 13 条](https://www.gnu.org/licenses/agpl-3.0.html#section13)，显著向这些用户提供免费获取该版本对应源码的途径。

上游源码位于[项目仓库](https://github.com/mujiajun-index/new-mcp)。部署修改版时，应通过可见的源码链接或其他合适方式，提供实际运行版本的对应源码，包括修改及所需的构建/安装脚本；仅链接到上游仓库不足以覆盖未公开的修改。许可声明和源码提供方式可参考 [GNU 官方应用指南](https://www.gnu.org/licenses/gpl-howto.en.html)。

使用本仓库 Dockerfile 构建的镜像包含 `/app/LICENSE`、`/app/NOTICE`、`/app/THIRD_PARTY_NOTICES.md` 和 `/app/licenses/system-one-connector/LICENSE`。第三方声明文件覆盖直接改编的源码，发布时还需保留适用的依赖许可声明。

如需为项目版权方有权授权的代码协商其他许可，可联系 jiajun25701@gmail.com。其他许可须与相关权利人另行达成协议，且不能免除第三方组件的许可义务。

---

## 💬 联系我们

- **QQ群**：`873292034`
- **邮箱**：jiajun25701@gmail.com

---

<div align="center">

如果这个项目对你有帮助，欢迎给个 ⭐️ Star！

**[GitHub](https://github.com/mujiajun-index/new-mcp)** • **[Issues](https://github.com/mujiajun-index/new-mcp/issues)** • **[在线体验](https://mcp.newmcp.pro/)**

</div>
