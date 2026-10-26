<div align="center">

![new-mcp](/web/public/logo.png)

# New MCP

🍥 **Next-Generation Unified MCP Service Management Platform**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <strong>English</strong>
</p>

<p align="center">
  <a href="https://mcp.newmcp.pro/">
    <img src="https://img.shields.io/badge/onlinedemo-mcp.newmcp.pro-success" alt="online demo">
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
| 📋 Service Registry | Register and test MCP services; discover tools, resources, resource templates, and prompts |
| 🗂️ Group Management | Independent endpoints, Direct/Smart modes, and per-group tool/resource/prompt filters |
| 🌐 MCP Gateway | Aggregate services authorized by each API Key through HTTP or WebSocket endpoints |
| 🔀 Protocol Bridging | Bridge between stdio, SSE, HTTP, WebSocket and passive connections |
| 🔑 Dual Auth | JWT user authentication + API Key for MCP client access |
| 🛡️ Role-Based Access | Super administrator, administrator, and user roles with user-group controls |
| 🔐 Upstream Key Pools | Single/multiple keys for SSE and Streamable HTTP; random/round-robin selection, header/query authentication, and automatic disabling on 401/403 |
| 📊 Operations | Call logs, health summaries, stdio process resource usage, and process start/stop/restart |

### 🤖 Transport Support

- **stdio** — Administrator-only registration/testing; runs MCP processes on the NewMCP host/container with `npx`, `uvx`, or custom commands
- **SSE** — Server-Sent Events
- **Streamable HTTP** — MCP `2026-07-28` requests and POST subscription streams, with legacy revisions through `2025-11-25` retained
- **WebSocket** — Client gateway connections and active upstream `ws://` / `wss://` connections
- **Passive WebSocket access** — Register a service to generate an endpoint; a local MCP server connects through its WebSocket bridge and publishes its tools

Passive endpoints use `ServerAddress` to generate a WS or WSS URL with an independent access credential. Each endpoint represents one MCP service; the latest successfully initialized connection replaces the previous one. The service page shows the endpoint, connection status, tool sync and credential reset. See [API](./docs/API.md) and [deployment](./docs/DEPLOY.md) for connection and proxy requirements.

### 🧠 Smart Discovery

- **Direct Mode** — Expose authorized tools from selected groups
- **Smart Mode** — Progressive BM25 catalog discovery, with optional `mcp.smart_search` powered by Jev/CLM/Laya for authorized user groups
- **Tool Namespace** — Automatic namespacing (`{ServiceName}__{toolName}`) to avoid conflicts

### ☁️ Device Integration

- Active connections to cloud platforms (XiaoZhi, etc.)
- WebSocket long connections for device control
- Camera and visual model integration support

### 🎨 Built-in AI Services

Create a configuration, test it, enable its virtual MCP service, and add that service to a group accessible by your API Key.

| Service | Capabilities |
|---------|--------------|
| Vision | Image analysis with `analyze_image`, plus `upload_image` for local reference images |
| Image Generation | OpenAI-compatible, Gemini, and OpenRouter protocols; text-to-image and image editing; persistent background tasks queried through `get_image_status` |
| Video Generation | OpenAI-compatible, Gemini, OpenRouter, and native MiniMax V2 (including compatible self-hosted Hailuo) protocols; text-to-video and single-image-to-video; persistent tasks queried through `get_video_status`, playback, download, and save retries |
| Cameras | Frame capture, frame analysis, and live camera streaming |
| System One | Jev/CLM/Laya decision models exposed through `evaluate`, with probability, choice, score, and batch evaluation |

Image/video generation returns a local `task_id` after a short wait. Query the status tool for that configuration to continue tracking it. Upstream credentials and model capabilities are configured separately for each service. See the [image](./docs/IMAGE-GENERATION.md), [video](./docs/VIDEO-GENERATION.md), and [decision model](./docs/SYSTEM-ONE.md) guides.

### 🛒 Marketplace, Billing, and Storage

- **Service marketplace** — Administrator-managed entries with categories/tags, reference-based installation, shared or dedicated stdio processes, and entry-level tool/resource/prompt pricing.
- **Optional billing** — Per-call marketplace billing, wallets, quotas, redemption codes, invitation rewards, user-group multipliers, and billing logs. Billing is disabled by default; user-owned services are free within NewMCP, while upstream providers may charge separately.
- **Media storage** — Local disk or S3-compatible storage, signed image URLs, direct uploads, per-user limits, retention rules, and automatic cleanup. Image/video retention can be set to `0` for permanent storage.

---

## 🏗 Architecture

```
┌─────────────┐     ┌─────────────────────────────┐     ┌──────────────┐
│  MCP Clients │────▶│       NewMCP Gateway         │────▶│ MCP Services │
│  (Claude,etc)│◀────│  /mcp  /mcp/group/{slug}    │◀────│ (stdio/SSE/  │
└─────────────┘     │        /smart/mcp            │     │  HTTP/WS)    │
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
| `POST /smart/mcp` | Streamable HTTP | Smart (fixed) | Aggregates authorized groups and exposes 5 core meta-tools, plus optional `mcp.smart_search` |
| `POST /mcp/group/{slug}` | Streamable HTTP | Per-group `expose_mode` | Endpoint-driven; each group configured independently |

Client gateway WebSocket routes (`/mcp/ws`, `/smart/mcp/ws`, `/mcp/ws/group/{slug}`) carry MCP JSON-RPC messages and subscription notifications. Upstream WebSocket registration and inbound `/mcp/passive/` access are also supported.

The gateway implements MCP revision `2026-07-28`: `server/discover`, per-request `_meta`, HTTP routing headers, `resultType`, and `subscriptions/listen`. Legacy clients can continue using `initialize` and GET SSE with `Mcp-Session-Id`. MCP revisions use date identifiers; JSON-RPC `2.0` and SDK versions are separate version numbers. NewMCP's WebSocket routes carry raw MCP JSON-RPC frames and require a compatible client or bridge.

An experimental Events API provides `events/list`, `events/poll`, `events/stream`, `events/subscribe`, and `events/unsubscribe` for four gateway change events: `mcp.tools.list_changed`, `mcp.resources.list_changed`, `mcp.prompts.list_changed`, and `mcp.resources.updated`. It supports polling, push, and signed HTTPS webhooks within the API Key's authorized scope. Webhook subscriptions live in memory for at most five minutes and require refresh; reconnect and resubscribe after a restart. This implementation covers gateway catalog/resource changes; upstream business events require additional integration. Smart mode acknowledges only the standard notification filters it can honor, while experimental events use the authorized service scope. See [protocol details and curl examples](./docs/MCP-PROTOCOL.md#11-订阅与实验-events) and [API reference](./docs/API.md#11-mcp-协议端点).

- **Direct mode** — exposes all tools at once. Suited for LLM clients with a large tool surface (Claude Code, Cursor).
- **Smart mode** — exposes 5 core meta-tools (`mcp.search` / `mcp.describe` / `mcp.execute` / `mcp.execute_batch` / `mcp.read`); the client discovers tools progressively via search → describe → execute, and runs independent calls concurrently through `mcp.execute_batch`. If an administrator enables semantic search for the caller's user group, `mcp.smart_search` is added. Suited for context-limited devices (e.g. XiaoZhi) or very large tool sets.
- For group endpoints (`/mcp/group/{slug}`), the mode is decided by each group's `expose_mode` setting (`direct` / `smart`).

### Tech Stack

| Layer | Technology |
|-------|-----------|
| **Backend** | Go 1.26.2+, Gin, GORM v2 |
| **Frontend** | React 19, TypeScript, Rsbuild, Radix UI, Tailwind CSS 4 |
| **Database** | SQLite (default), MySQL, PostgreSQL |
| **Cache** | In-memory caches and session pools |
| **Storage** | Local disk, S3-compatible object storage |
| **Auth** | JWT, API Key |
| **Protocol** | MCP (Model Context Protocol) |

### Project Structure

```
├── cmd/server/        — Application entry point
├── router/            — HTTP routing
├── controller/        — Request handlers
├── service/           — Business logic
├── model/             — Data models (GORM)
├── middleware/        — Auth, rate limiting, CORS
├── billing/           — Marketplace pricing and quota settlement
├── internal/          — Internal packages (MCP adapter, gateway)
├── common/            — Shared utilities
├── dto/               — Data transfer objects
├── frontend.go        — Embed generated web/dist assets
├── web/               — React frontend
├── scripts/           — Migration and verification utilities
└── docs/              — Documentation
```

---

## 🚀 Quick Start

### Start with Docker Compose

Requires Docker with the Compose plugin. The default deployment uses the published image and SQLite; no local Go or Node.js installation is needed.

```bash
# Clone the project
git clone https://github.com/mujiajun-index/new-mcp.git
cd new-mcp

# Before starting, edit docker-compose.yaml and replace SESSION_SECRET and
# CRYPTO_SECRET with two independent random strings of at least 32 characters.
docker compose up -d
```

Open `http://localhost:3000`. The first visit opens `/setup`, where you create the super administrator account (password: at least 8 characters); no default administrator credentials are shipped. Data and logs are persisted in `./data` and `./logs`.

### Connect Your First MCP Client

1. Sign in and set **System Settings → Server Address** (`ServerAddress`) to the address clients can reach. For remote access, use your public HTTPS address.
2. Register and test an MCP service, install a marketplace entry, or enable a built-in AI configuration.
3. Create a group, add the service, and enable the tools/resources/prompts you want to expose.
4. Create an API Key and authorize that group (or all your groups). An API Key without authorized groups exposes no services.
5. Copy the connection configuration from the group page into your MCP client and replace the placeholder key. For example, for a client accepting `mcpServers` with Streamable HTTP:

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

Replace `my-group` with your group identifier. Use `/mcp` for all groups authorized by the key or `/smart/mcp` for Smart discovery. The gateway accepts `X-API-Key` or `Authorization: Bearer sk-...`; the web login JWT is for management APIs. Client configuration fields vary, so adapt the example to your client.

### Run from Source

Requirements: **Go 1.26.2+**, **Node.js 22.13+ (22.x) or 24+**, npm, and optionally Make. The Node.js versions cover both the frontend build and lint dependencies in the lockfile. Running stdio services also requires their commands/runtimes on the backend host; the Docker image includes Node.js/npm and Python, and can install `uv` when needed.

From the cloned repository:

```bash
cp .env.example .env
# Edit .env: set independent SESSION_SECRET and CRYPTO_SECRET values.

# Build the frontend before compiling the backend.
cd web
npm ci
npm run build
cd ..

make run
# Without Make: go run ./cmd/server/
```

The backend loads `.env` from its working directory and serves the embedded frontend on `http://localhost:3000`. A fresh checkout needs `web/dist` generated before Go build/run/test because `frontend.go` uses `go:embed`. Rebuild the frontend and recompile/restart the backend after frontend changes when using the embedded UI.

On Windows PowerShell, use `Copy-Item .env.example .env`; use `go run ./cmd/server/` if Make is unavailable.

### Frontend Development

```bash
cd web
npm ci
npm run dev
```

Keep the Go backend running on port `3000` and open `http://localhost:5173`. Rsbuild proxies `/api/`, `/v/`, and `/mcp/` to the backend; use port `3000` (or your deployed gateway address) for MCP client connections.

---

## 🚢 Deployment

### Build

```bash
# Build web/dist first as described above, then build the binary.
make build

# The output binary is in build/newmcp
```

### Run

```bash
./build/newmcp
```

Run from the directory containing your `.env` and persistent data. Without Make, use `go build -o build/newmcp ./cmd/server/`; on Windows, build with `-o build/newmcp.exe` and run `.\build\newmcp.exe`. Startup automatically migrates the database schema. Back up the database, stored media, and secrets before upgrading.

### Docker

Pull and run with Docker (frontend and backend on port 3000):

```bash
# Pull image
docker pull mujkjk/new-mcp:latest

# Copy .env.example to .env and set both secrets before running.
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

The command above uses Bash syntax; Compose is convenient across platforms. To build the full image from local source, run `docker build -t new-mcp:local .`.

#### Environment Variables

| Variable | Default (source / Docker image) | Description |
|----------|--------------------------------|-------------|
| `PORT` | `3000` | Server port |
| `GIN_MODE` | `debug` / `release` | Gin mode (`debug`/`release`) |
| `DB_TYPE` | `sqlite` | Database type (`sqlite`/`mysql`/`postgres`) |
| `DB_PATH` | `./data/newmcp.db` / `/app/data/newmcp.db` | SQLite database path |
| `SQL_DSN` | — | MySQL/PostgreSQL connection string |
| `SESSION_SECRET` | Unset | JWT and signed URL secret; set a strong, stable value before first use |
| `CRYPTO_SECRET` | Unset | Sensitive-data encryption key; keep stable so stored credentials remain readable |
| `LOG_LEVEL` | `info` | Log level: `debug` / `info` / `warn` / `error` |
| `TZ` | Host/container timezone | Process timezone; examples use `Asia/Shanghai` |
| `REDIS_CONN_STRING` | Unset | Reserved configuration; currently read but no Redis client/cache is connected |

File logs default to `./logs`. Set the CLI flag `--log-dir /path/to/logs` to change the directory, or `--log-dir ""` for console-only logging. Keep both secrets non-empty: without `SESSION_SECRET`, signing uses a built-in fallback; without `CRYPTO_SECRET`, sensitive data uses base64 encoding rather than encryption.

> [!NOTE]
> MCP endpoint URLs are derived from the **Server Address** (`ServerAddress`) in **System Settings**, which admins can change at runtime — no restart or environment variable required.

Storage, retention, SMTP, registration, billing, rate limiting, and semantic search are also managed in the admin UI and database, rather than as arbitrary environment variables. When using local media storage, configure an address reachable by clients and any upstream provider that fetches reference images. Changes to the storage backend/connection settings take effect after restarting NewMCP.

#### Using MySQL / PostgreSQL

Set `DB_TYPE` and `SQL_DSN` in the native `.env`, the Docker `--env-file`, or Compose's `environment` section. Use an existing database and credentials:

```dotenv
# MySQL
DB_TYPE=mysql
SQL_DSN=newmcp:password@tcp(mysql-host:3306)/newmcp?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai

# PostgreSQL (use these instead of the MySQL values)
# DB_TYPE=postgres
# SQL_DSN=host=postgres-host port=5432 user=newmcp password=password dbname=newmcp sslmode=disable
```

Choose PostgreSQL TLS settings appropriate to your database. Inside Docker, database hostnames must be reachable from the application container. Changing the database type does not migrate existing data; [scripts/sqlite_to_mysql.py](./scripts/sqlite_to_mysql.py) is available for SQLite-to-MySQL migration.

#### Docker Compose

> [!TIP]
> The checked-in [docker-compose.yaml](./docker-compose.yaml) uses `image: mujkjk/new-mcp:latest`. Replace its two secret placeholders before starting; it does not pass the repository's `.env` to the container automatically.

```bash
# Pull the image and start in the background
docker compose pull
docker compose up -d

# View logs
docker compose logs -f new-mcp

# Stop
docker compose down

# Update to the latest published image
docker compose pull
docker compose up -d
```

The default bind mounts are **`./data:/app/data`** and **`./logs:/app/logs`**. SQLite and local media persist under `./data`; stopping/removing the container with `docker compose down` retains these host directories.

For a source build, uncomment `build: .` and replace the service image with a local tag such as `new-mcp:local`, then run `docker compose up -d --build`. The default image-only configuration does not rebuild local changes.

Optional MySQL/PostgreSQL service templates are included. Enable the matching database service, named volume, `DB_TYPE`, `SQL_DSN`, and dependency/health check; match the DSN credentials to the database user's configuration. Redis is not required by the current implementation.

For reverse proxies, enable WebSocket upgrades, disable buffering for SSE, and allow long-lived requests. Proxy the complete application, including `/mcp`, `/smart/mcp`, `/mcp/passive/`, `/api/`, `/u/`, and `/v/`. Set `ServerAddress` to the external application origin; browser MCP integrations from additional origins require `MCPAllowedOrigins`. Check the running server with:

```bash
curl http://localhost:3000/health
# {"status":"ok","version":"..."}
```

### Available Make Commands

| Command | Description |
|---------|-------------|
| `make build` | Build the Go binary; requires generated `web/dist` |
| `make run` | Build and run the backend with embedded frontend assets |
| `make dev` | Run the backend with `go run`; no automatic reload or frontend build |
| `make test` | Run Go tests (`go test ./... -v`); requires generated `web/dist` |
| `make tidy` | Tidy Go modules |
| `make clean` | Delete **both `build/` and `data/`**, including local database/media; back up first |

Frontend checks (run inside `web/`): `npm run typecheck`, `npm run lint`, and `npm run build:check`. Focused frontend utility tests: `npm run test:videogen-params`, `npm run test:task-history`, and `npm run test:datetime`.

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
| [Image Generation](docs/IMAGE-GENERATION.md) | Providers, editing, async tasks, and result storage |
| [Video Generation](docs/VIDEO-GENERATION.md) | Providers, text/image-to-video, task tracking, and downloads |
| [System One](docs/SYSTEM-ONE.md) | Decision model configuration and semantic search |
| [Media Uploads and Storage](docs/VISION-IMAGE-TRANSPORT.md) | Image uploads, signed URLs, S3, retention, and quotas |
| [Camera Agent Calls](docs/CAMERA-AGENT-CALLING.md) | Camera capture and analysis workflows |
| [Multiple Upstream Keys](docs/MULTI-KEY.md) | Key pools, selection strategies, and authentication |
| [Billing Design](docs/COMMERCIALIZATION.md) | Marketplace pricing, wallets, quotas, and settlement |

The detailed documents include historical plans and draft designs; use the commands and defaults in this README together with the checked-in configuration/source when they differ.

---

## 🌟 Star History

<div align="center">

[![Star History Chart](https://api.star-history.com/chart?repos=mujiajun-index/new-mcp&type=date&legend=top-left)](https://www.star-history.com/?type=date&repos=mujiajun-index%2Fnew-mcp)

</div>

---

## 📜 License

Unless a file states otherwise, NewMCP is licensed under the [GNU Affero General Public License, version 3 or any later version](./LICENSE) (`AGPL-3.0-or-later`). The project's copyright and license grant are in [NOTICE](./NOTICE); third-party components retain their own licenses, including the MIT-licensed code identified in [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md).

Commercial use is permitted under the license. When distributing source, binaries, or container images, comply with the applicable notice and Corresponding Source requirements. If you modify NewMCP and let users interact with that version over a network, prominently offer those users access to its Corresponding Source at no charge, as required by [AGPLv3 section 13](https://www.gnu.org/licenses/agpl-3.0.html#section13).

The upstream source is available in the [project repository](https://github.com/mujiajun-index/new-mcp). Deployments of modified versions should provide a visible source link or another suitable source-access mechanism for the version actually running, including its modifications and required build/install scripts; an upstream-only link does not cover unpublished changes. See [GNU's application guide](https://www.gnu.org/licenses/gpl-howto.en.html) for notice and source-offer guidance.

Docker images built with this repository's Dockerfile include `/app/LICENSE`, `/app/NOTICE`, `/app/THIRD_PARTY_NOTICES.md`, and `/app/licenses/system-one-connector/LICENSE`. The third-party notice file covers directly adapted source; release maintainers must also retain applicable dependency notices.

To discuss alternative licensing for code controlled by the project's copyright holders, contact jiajun25701@gmail.com. Alternative terms require a separate agreement with the relevant rights holders and do not remove third-party license obligations.

---

## 💬 Contact

- **QQGroup**: `873292034`
- **Email**: jiajun25701@gmail.com

---

<div align="center">

If this project is helpful to you, welcome to give a ⭐️ Star!

**[GitHub](https://github.com/mujiajun-index/new-mcp)** • **[Issues](https://github.com/mujiajun-index/new-mcp/issues)** • **[Online Demo](https://mcp.newmcp.pro/)**

</div>
