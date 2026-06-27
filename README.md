<div align="center">

![new-mcp](/web/public/logo.png)

# NewMCP

**Next-Generation Unified MCP Service Management Platform**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <strong>English</strong>
</p>

<p align="center">
  <a href="https://raw.githubusercontent.com/mujiajun-index/new-mcp/main/LICENSE">
    <img src="https://img.shields.io/github/license/mujiajun-index/new-mcp?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://github.com/mujiajun-index/new-mcp/releases/latest">
    <img src="https://img.shields.io/github/v/release/mujiajun-index/new-mcp?color=brightgreen&include_prereleases" alt="release">
  </a><!--
  --><a href="https://hub.docker.com/r/mujkjk/new-mcp">
    <img src="https://img.shields.io/badge/docker-dockerHub-blue" alt="docker">
  </a><!--
  --><a href="https://goreportcard.com/report/github.com/mujiajun-index/new-mcp">
    <img src="https://goreportcard.com/badge/github.com/mujiajun-index/new-mcp" alt="GoReportCard">
  </a>
</p>

<p align="center">
  <a href="#-quick-start">Quick Start</a> •
  <a href="#-key-features">Key Features</a> •
  <a href="#-architecture">Architecture</a> •
  <a href="#-deployment">Deployment</a>
</p>

</div>

---

## 📝 Project Description

NewMCP is a unified MCP (Model Context Protocol) service management platform. It provides service registration, grouping, routing, and protocol bridging — enabling you to manage all your MCP services in one place and expose them through a unified gateway endpoint.

> [!NOTE]
> This project is for personal learning and research purposes. It is designed for developers who want to self-host and manage MCP services.

---

## ✨ Key Features

### 🎯 Core Functions

| Feature | Description |
|---------|-------------|
| 📋 Service Registry | Register MCP services with automatic tool discovery via `tools/list` |
| 🗂️ Group Management | Organize services into groups with independent MCP endpoints |
| 🌐 MCP Gateway | Unified Streamable HTTP and WebSocket endpoints for all services |
| 🔀 Protocol Bridging | Bridge between stdio, SSE, HTTP, WebSocket and passive connections |
| 🔑 Dual Auth | JWT user authentication + API Key for MCP client access |
| 🛡️ Role-Based Access | Admin/User role management with permission controls |

### 🤖 Transport Support

- **stdio** — Standard input/output transport
- **SSE** — Server-Sent Events
- **Streamable HTTP** — HTTP-based MCP transport
- **WebSocket** — Full-duplex WebSocket connections
- **Passive** — Client-initiated connections (e.g., XiaoZhi devices)

### 🧠 Smart Discovery

- **Direct Mode** — Expose all tools from registered services
- **Smart Mode** — Progressive tool discovery via BM25 search algorithm
- **Tool Namespace** — Automatic namespacing (`{ServiceName}__{toolName}`) to avoid conflicts

### ☁️ Device Integration

- Active connections to cloud platforms (XiaoZhi, etc.)
- WebSocket long connections for device control
- Camera and visual model integration support

---

## 🏗 Architecture

```
┌─────────────┐     ┌─────────────────────────────┐     ┌──────────────┐
│  MCP Clients │────▶│       NewMCP Gateway         │────▶│ MCP Services │
│  (Claude,etc)│◀────│  /mcp  /mcp/group/{slug}    │◀────│ (stdio/SSE/  │
└─────────────┘     │  /mcp/ws  /mcp/ws/group/...  │     │  HTTP/WS)    │
                    └─────────────────────────────┘     └──────────────┘
                           │
                    ┌──────┴──────┐
                    │   Backend   │
                    │  Go + Gin   │
                    │  GORM + DB  │
                    └──────┬──────┘
                           │
                    ┌──────┴──────┐
                    │  Frontend   │
                    │ React + TS  │
                    └─────────────┘
```

### MCP Gateway

NewMCP exposes MCP tools through a unified gateway supporting two **tool exposure modes**, driven by endpoint routing:

| Endpoint | Transport | Mode | Description |
|----------|-----------|------|-------------|
| `POST /mcp` | Streamable HTTP | Direct (fixed) | Aggregates all groups bound to the API Key, dedupes and exposes every tool (`serviceName__toolName`) |
| `POST /smart/mcp` | Streamable HTTP | Smart (fixed) | Aggregates all groups, exposes only 3 meta-tools for progressive discovery |
| `POST /mcp/group/{slug}` | Streamable HTTP | Per-group `expose_mode` | Endpoint-driven; each group configured independently |
| `GET /mcp/ws` | WebSocket | Direct (fixed) | Same as `POST /mcp` |
| `GET /smart/mcp/ws` | WebSocket | Smart (fixed) | Same as `POST /smart/mcp` |
| `GET /mcp/ws/group/{slug}` | WebSocket | Per-group `expose_mode` | Endpoint-driven |

- **Direct mode** — exposes all tools at once. Suited for LLM clients with a large tool surface (Claude Code, Cursor).
- **Smart mode** — exposes only 3 meta-tools (`mcp.search` / `mcp.describe` / `mcp.execute`); the client discovers tools progressively via search → describe → execute. Suited for context-limited devices (e.g. XiaoZhi) or very large tool sets.
- For group endpoints (`/mcp/group/{slug}`), the mode is decided by each group's `expose_mode` setting (`direct` / `smart`).

### Tech Stack

| Layer | Technology |
|-------|-----------|
| **Backend** | Go 1.26+, Gin, GORM v2 |
| **Frontend** | React 19, TypeScript, Rsbuild, Radix UI, Tailwind CSS 4 |
| **Database** | SQLite (default), MySQL, PostgreSQL |
| **Cache** | Redis (optional), in-memory cache |
| **Auth** | JWT, API Key |
| **Protocol** | MCP (Model Context Protocol) |

### Project Structure

```
├── cmd/server/        — Application entry point
├── router/            — HTTP routing
├── controller/        — Request handlers
├── service/           — Business logic
├── model/             — Data models (GORM)
├── middleware/         — Auth, rate limiting, CORS
├── internal/          — Internal packages (MCP adapter, gateway)
├── common/            — Shared utilities
├── dto/               — Data transfer objects
├── constant/          — Constants
├── web/               — React frontend
└── docs/              — Documentation
```

---

## 🚀 Quick Start

### Prerequisites

- Go 1.26+
- Node.js 18+ (for frontend development)

### Build & Run

```bash
# Clone the project
git clone https://github.com/mujiajun-index/new-mcp.git
cd new-mcp

# Build and run
make run
```

Or run directly in development mode:

```bash
make dev
```

The service will start on `http://localhost:3000` by default.

### Frontend Development

```bash
cd web
npm install     # or bun install
npm run dev     # or bun run dev
```

---

## 🚢 Deployment

### Build

```bash
# Build the binary
make build

# The output binary is in build/newmcp
```

### Run

```bash
./build/newmcp
```

### Docker

Pull and run with Docker (frontend and backend on port 3000):

```bash
# Pull image
docker pull mujkjk/new-mcp:latest

# Run container
docker run --name new-mcp -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v /www/wwwroot/newmcp:/app/data \
  mujkjk/new-mcp:latest
```

#### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `3000` | Server port |
| `GIN_MODE` | `release` | Gin mode (`debug`/`release`) |
| `DB_TYPE` | `sqlite` | Database type (`sqlite`/`mysql`/`postgres`) |
| `DB_PATH` | `/app/data/newmcp.db` | SQLite database path |
| `SQL_DSN` | — | MySQL/PostgreSQL connection string |
| `REDIS_CONN_STRING` | — | Redis connection string (optional) |
| `SESSION_SECRET` | — | JWT session secret |
| `CRYPTO_SECRET` | — | Encryption key for sensitive data |

> [!NOTE]
> MCP endpoint URLs are derived from the **Server Address** (`ServerAddress`) in **System Settings**, which admins can change at runtime — no restart or environment variable required.

#### Using MySQL / PostgreSQL

```bash
docker run --name new-mcp -d --restart always \
  -p 3000:3000 \
  -e DB_TYPE=mysql \
  -e SQL_DSN="user:password@tcp(mysql-host:3306)/newmcp" \
  -e TZ=Asia/Shanghai \
  mujkjk/new-mcp:latest
```

#### Docker Compose

> [!TIP]
> The project includes a ready-to-use `docker-compose.yaml`. You can start all services with a single command:

```bash
# Start (build & run in background)
docker compose up -d

# View logs
docker compose logs -f

# Stop
docker compose down

# Rebuild after code changes
docker compose up -d --build
```

The default configuration uses **SQLite** with persistent data in a Docker volume. To switch to **MySQL**, **PostgreSQL**, or add **Redis**, edit `docker-compose.yaml` and uncomment the corresponding sections.

<details>
<summary>📝 docker-compose.yaml (default SQLite)</summary>

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

volumes:
  newmcp-data:
```

</details>

### Available Make Commands

| Command | Description |
|---------|-------------|
| `make build` | Build the Go binary |
| `make run` | Build and run |
| `make dev` | Run in dev mode (go run) |
| `make test` | Run tests |
| `make tidy` | Tidy Go modules |
| `make clean` | Clean build artifacts |

---

## 📚 Documentation

| Document | Description |
|----------|-------------|
| [PRD](docs/PRD.md) | Product Requirements Document |
| [Architecture](docs/ARCHITECTURE.md) | System Architecture Design |
| [API](docs/API.md) | API Documentation |
| [Database](docs/DATABASE.md) | Database Schema |
| [MCP Protocol](docs/MCP-PROTOCOL.md) | MCP Protocol Details |
| [Deployment](docs/DEPLOY.md) | Deployment Guide |
| [Frontend](docs/FRONTEND.md) | Frontend Design |
| [XiaoZhi Integration](docs/XIAOZHI-INTEGRATION.md) | XiaoZhi Device Integration |

---

## 🌟 Star History

<div align="center">

[![Star History Chart](https://api.star-history.com/chart?repos=mujiajun-index/new-mcp&type=date&legend=top-left)](https://www.star-history.com/?type=date&repos=mujiajun-index%2Fnew-mcp)

</div>

---

## 📜 License

This project is licensed under the [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE).

If your organization's policies do not permit the use of AGPLv3-licensed software, or if you wish to avoid the open-source obligations of AGPLv3 for commercial use, please contact: jiajun25701@gmail.com

---

## 💬 Contact

- **QQGroup**: `873292034`
- **Email**: jiajun25701@gmail.com

---

<div align="center">

If this project is helpful to you, welcome to give a ⭐️ Star!

**[GitHub](https://github.com/mujiajun-index/new-mcp)** • **[Issues](https://github.com/mujiajun-index/new-mcp/issues)**

</div>
