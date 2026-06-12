<div align="center">

# NewMCP

**下一代统一 MCP 服务管理平台**

<p align="center">
  <strong>简体中文</strong> |
  <a href="./README.md">English</a>
</p>

<p align="center">
  <a href="https://github.com/mujiajun-index/new-mcp">
    <img src="https://img.shields.io/github/license/mujiajun-index/new-mcp?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://github.com/mujiajun-index/new-mcp/releases/latest">
    <img src="https://img.shields.io/github/v/release/mujiajun-index/new-mcp?color=brightgreen&include_prereleases" alt="release">
  </a><!--
  --><a href="https://goreportcard.com/report/github.com/mujiajun-index/new-mcp">
    <img src="https://goreportcard.com/badge/github.com/mujiajun-index/new-mcp" alt="GoReportCard">
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
| 📋 服务注册 | 注册 MCP 服务，通过 `tools/list` 自动发现工具 |
| 🗂️ 分组管理 | 将服务组织到分组中，每个分组拥有独立的 MCP 端点 |
| 🌐 MCP 网关 | 统一的 Streamable HTTP 和 WebSocket 端点，聚合所有服务 |
| 🔀 协议桥接 | 在 stdio、SSE、HTTP、WebSocket 和被动连接之间桥接 |
| 🔑 双重认证 | JWT 用户认证 + API Key 用于 MCP 客户端访问 |
| 🛡️ 角色权限 | 管理员/用户角色管理与权限控制 |

### 🤖 传输协议支持

- **stdio** — 标准输入/输出传输
- **SSE** — 服务端推送事件
- **Streamable HTTP** — 基于 HTTP 的 MCP 传输
- **WebSocket** — 全双工 WebSocket 连接
- **被动连接** — 客户端主动连接（如小智设备）

### 🧠 智能发现

- **直连模式** — 暴露已注册服务的所有工具
- **智能模式** — 通过 BM25 搜索算法渐进式发现工具
- **工具命名空间** — 自动命名隔离（`{服务名}__{工具名}`），避免冲突

### ☁️ 设备集成

- 主动连接云平台（小智等）
- WebSocket 长连接设备控制
- 摄像头与视觉模型集成支持

---

## 🏗 系统架构

```
┌─────────────┐     ┌─────────────────────────────┐     ┌──────────────┐
│  MCP 客户端  │────▶│       NewMCP 网关            │────▶│  MCP 服务    │
│ (Claude等)  │◀────│  /mcp  /mcp/group/{slug}    │◀────│ (stdio/SSE/  │
└─────────────┘     │  /mcp/ws  /mcp/ws/group/...  │     │  HTTP/WS)    │
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

### 技术栈

| 层级 | 技术 |
|------|------|
| **后端** | Go 1.26+, Gin, GORM v2 |
| **前端** | React 19, TypeScript, Rsbuild, Radix UI, Tailwind CSS 4 |
| **数据库** | SQLite（默认）, MySQL, PostgreSQL |
| **缓存** | Redis（可选）, 内存缓存 |
| **认证** | JWT, API Key |
| **协议** | MCP (Model Context Protocol) |

### 项目结构

```
├── cmd/server/        — 应用入口
├── router/            — HTTP 路由
├── controller/        — 请求处理
├── service/           — 业务逻辑
├── model/             — 数据模型 (GORM)
├── middleware/         — 认证、限流、CORS
├── internal/          — 内部包（MCP 适配器、网关）
├── common/            — 公共工具
├── dto/               — 数据传输对象
├── constant/          — 常量定义
├── web/               — React 前端
└── docs/              — 项目文档
```

---

## 🚀 快速开始

### 环境要求

- Go 1.26+
- Node.js 18+（前端开发需要）

### 构建与运行

```bash
# 克隆项目
git clone https://github.com/mujiajun-index/new-mcp.git
cd new-mcp

# 构建并运行
make run
```

或直接以开发模式运行：

```bash
make dev
```

服务默认启动在 `http://localhost:3000`。

### 前端开发

```bash
cd web
npm install     # 或 bun install
npm run dev     # 或 bun run dev
```

---

## 🚢 部署

### 构建

```bash
# 编译二进制文件
make build

# 输出文件位于 build/newmcp
```

### 运行

```bash
./build/newmcp
```

### Docker

使用 Docker 构建并运行（前端和后端均运行在 3000 端口）：

```bash
# 构建镜像
docker build -t newmcp .

# 运行容器
docker run --name new-mcp -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v /home/data/newmcp:/app/data \
  newmcp
```

#### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PORT` | `3000` | 服务端口 |
| `GIN_MODE` | `release` | Gin 模式（`debug`/`release`） |
| `DB_TYPE` | `sqlite` | 数据库类型（`sqlite`/`mysql`/`postgres`） |
| `DB_PATH` | `/app/data/newmcp.db` | SQLite 数据库路径 |
| `SQL_DSN` | — | MySQL/PostgreSQL 连接字符串 |
| `REDIS_CONN_STRING` | — | Redis 连接字符串（可选） |
| `SESSION_SECRET` | — | JWT 会话密钥 |
| `CRYPTO_SECRET` | — | 敏感数据加密密钥 |
| `BASE_URL` | `http://localhost:3000` | 外部访问地址 |

#### 使用 MySQL / PostgreSQL

```bash
docker run --name new-mcp -d --restart always \
  -p 3000:3000 \
  -e DB_TYPE=mysql \
  -e "SQL_DSN=user:password@tcp(mysql-host:3306)/newmcp" \
  -e TZ=Asia/Shanghai \
  -e SESSION_SECRET=your-secret \
  -e CRYPTO_SECRET=your-crypto-key \
  newmcp
```

#### Docker Compose

> [!TIP]
> 项目自带 `docker-compose.yaml` 文件，一条命令即可启动所有服务：

```bash
# 启动（构建并在后台运行）
docker compose up -d

# 查看日志
docker compose logs -f

# 停止
docker compose down

# 代码修改后重新构建
docker compose up -d --build
```

默认配置使用 **SQLite**，数据通过 Docker 卷持久化。如需切换为 **MySQL**、**PostgreSQL** 或启用 **Redis**，编辑 `docker-compose.yaml` 取消对应部分的注释即可。

<details>
<summary>📝 docker-compose.yaml（默认 SQLite）</summary>

```yaml
version: "3.8"
services:
  newmcp:
    build: .
    container_name: new-mcp
    restart: always
    ports:
      - "3000:3000"
    volumes:
      - newmcp-data:/app/data
    environment:
      - TZ=Asia/Shanghai
      - GIN_MODE=release
      - DB_TYPE=sqlite
      - DB_PATH=/app/data/newmcp.db
      - SESSION_SECRET=change-me-to-a-random-string
      - CRYPTO_SECRET=change-me-to-a-random-string
      - BASE_URL=http://localhost:3000

volumes:
  newmcp-data:
```

</details>

### Make 命令一览

| 命令 | 说明 |
|------|------|
| `make build` | 编译 Go 二进制文件 |
| `make run` | 编译并运行 |
| `make dev` | 开发模式运行 (go run) |
| `make test` | 运行测试 |
| `make tidy` | 整理 Go 模块 |
| `make clean` | 清理构建产物 |

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

---

## 🌟 Star 历史

[![Star History Chart](https://api.star-history.com/svg?repos=mujiajun-index/new-mcp&type=Date)](https://star-history.com/#mujiajun-index/new-mcp&Date)

---

## 📜 许可证

本项目基于 [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE) 开源协议。

如果您的组织不允许使用 AGPLv3 授权的软件，或希望避免 AGPLv3 的开源义务以用于商业用途，请联系：jiajun25701@gmail.com

---

<div align="center">

如果这个项目对你有帮助，欢迎给个 ⭐️ Star！

**[GitHub](https://github.com/mujiajun-index/new-mcp)** • **[Issues](https://github.com/mujiajun-index/new-mcp/issues)**

</div>
