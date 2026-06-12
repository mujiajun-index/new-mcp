<div align="center">

![new-mcp](/web/public/logo.png)

# NewMCP

**Next-Generation Unified MCP Service Management Platform**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <strong>English</strong>
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

Build and run with Docker (frontend and backend on port 3000):

```bash
# Build image
docker build -t newmcp .

# Run container
docker run --name new-mcp -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v /home/data/newmcp:/app/data \
  newmcp
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
| `BASE_URL` | `http://localhost:3000` | External base URL |

#### Using MySQL / PostgreSQL

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

```yaml
version: "3.8"
services:
  newmcp:
    build: .
    restart: always
    ports:
      - "3000:3000"
    volumes:
      - newmcp-data:/app/data
    environment:
      - TZ=Asia/Shanghai
      - SESSION_SECRET=your-session-secret
      - CRYPTO_SECRET=your-crypto-secret

volumes:
  newmcp-data:
```

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

[![Star History Chart](https://api.star-history.com/svg?repos=mujiajun-index/new-mcp&type=Date)](https://star-history.com/#mujiajun-index/new-mcp&Date)

---

## 📜 License

This project is licensed under the [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE).

If your organization's policies do not permit the use of AGPLv3-licensed software, or if you wish to avoid the open-source obligations of AGPLv3 for commercial use, please contact: jiajun25701@gmail.com

---

<div align="center">

If this project is helpful to you, welcome to give a ⭐️ Star!

**[GitHub](https://github.com/mujiajun-index/new-mcp)** • **[Issues](https://github.com/mujiajun-index/new-mcp/issues)**

</div>
