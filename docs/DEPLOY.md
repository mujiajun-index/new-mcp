# NewMCP 部署指南

> 版本: V1.0 | 状态: 草案 | 更新日期: 2026-05-03

## 1. 开发环境搭建

### 1.1 前置依赖

| 依赖 | 版本要求 | 说明 |
|------|----------|------|
| Go | >= 1.22 | 后端开发 |
| Node.js | >= 18 | 前端开发 |
| pnpm / bun | latest | 前端包管理 |
| Docker | >= 20 | 可选，用于 MySQL/Redis |
| Git | latest | 版本控制 |

### 1.2 克隆项目

```bash
git clone https://github.com/your-org/newmcp.git
cd newmcp
```

### 1.3 启动依赖服务 (Docker)

```bash
# 启动 MySQL 和 Redis
docker compose -f docker-compose.dev.yml up -d
```

`docker-compose.dev.yml` 内容：
```yaml
version: '3.8'
services:
  mysql:
    image: mysql:8.0
    ports:
      - "3306:3306"
    environment:
      MYSQL_ROOT_PASSWORD: root
      MYSQL_DATABASE: newmcp_dev
      MYSQL_USER: newmcp
      MYSQL_PASSWORD: newmcp
    volumes:
      - mysql_dev_data:/var/lib/mysql

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

volumes:
  mysql_dev_data:
```

### 1.4 后端开发

```bash
# 安装 Go 依赖
go mod tidy

# 配置环境变量
cp .env.example .env
# 编辑 .env 配置数据库连接等

# 启动热重载开发服务器 (需要安装 air)
go install github.com/air-verse/air@latest
air -c .air.toml
```

`.env.example`:
```bash
# 服务配置
PORT=3000
GIN_MODE=debug

# 数据库 (三选一)
# SQLite (默认)
DB_TYPE=sqlite
DB_PATH=./data/newmcp.db

# MySQL
# DB_TYPE=mysql
# SQL_DSN=newmcp:newmcp@tcp(localhost:3306)/newmcp?charset=utf8mb4&parseTime=True

# PostgreSQL
# DB_TYPE=postgres
# SQL_DSN=host=localhost port=5432 user=newmcp password=newmcp dbname=newmcp

# Redis (可选)
# REDIS_CONN_STRING=redis://localhost:6379

# 安全
SESSION_SECRET=your-session-secret-change-me
CRYPTO_SECRET=your-crypto-secret-change-me

# 日志
LOG_LEVEL=debug
```

`.air.toml` (热重载配置):
```toml
root = "."
tmp_dir = "tmp"

[build]
  bin = "./tmp/main"
  cmd = "go build -o ./tmp/main ./cmd/server/"
  delay = 1000
  exclude_dir = ["web", "tmp", "vendor", "data"]
  exclude_regex = ["_test.go"]
  include_ext = ["go", "toml", "yaml", "json"]
  kill_delay = "0.5s"
  send_interrupt = true
```

### 1.5 前端开发

```bash
cd web

# 安装依赖
pnpm install

# 启动开发服务器 (http://localhost:5173)
pnpm dev
```

Vite 配置代理到后端：

`web/vite.config.ts`:
```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:3000',
        changeOrigin: true,
      },
      '/mcp': {
        target: 'http://localhost:3000',
        ws: true,
        changeOrigin: true,
      },
    },
  },
})
```

---

## 2. 生产部署

### 2.1 Docker Compose 部署 (推荐)

```bash
# 构建并启动
docker compose up -d
```

`docker-compose.yml`:
```yaml
version: '3.8'

services:
  newmcp:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: newmcp
    ports:
      - "3000:3000"
    environment:
      - DB_TYPE=mysql
      - SQL_DSN=newmcp:newmcp@tcp(mysql:3306)/newmcp?charset=utf8mb4&parseTime=True
      - REDIS_CONN_STRING=redis://redis:6379
      - SESSION_SECRET=${SESSION_SECRET}
      - CRYPTO_SECRET=${CRYPTO_SECRET}
      - PORT=3000
      - GIN_MODE=release
    volumes:
      - ./data:/data
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_started
    restart: unless-stopped

  mysql:
    image: mysql:8.0
    container_name: newmcp-mysql
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD}
      MYSQL_DATABASE: newmcp
      MYSQL_USER: newmcp
      MYSQL_PASSWORD: newmcp
    volumes:
      - mysql_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 3
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    container_name: newmcp-redis
    volumes:
      - redis_data:/data
    restart: unless-stopped

volumes:
  mysql_data:
  redis_data:
```

### 2.2 Dockerfile

```dockerfile
# ====== Build Stage ======
FROM golang:1.22-alpine AS backend-builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /newmcp ./cmd/server/

# ====== Frontend Build Stage ======
FROM node:20-alpine AS frontend-builder

WORKDIR /app/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN npm install -g pnpm && pnpm install --frozen-lockfile

COPY web/ .
RUN pnpm build

# ====== Final Stage ======
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# 从构建阶段复制二进制
COPY --from=backend-builder /newmcp /app/newmcp

# 从前端构建阶段复制静态文件
COPY --from=frontend-builder /app/web/dist /app/web/dist

# 创建数据目录
RUN mkdir -p /data

EXPOSE 3000

ENTRYPOINT ["/app/newmcp"]
```

### 2.3 单二进制部署

```bash
# 构建
make build

# 运行 (使用 SQLite，无需外部依赖)
./newmcp

# 或指定配置
./newmcp --port 3000 --db sqlite:///data/newmcp.db
```

### 2.4 环境变量参考

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PORT` | `3000` | 服务端口 |
| `GIN_MODE` | `debug` | Gin 模式: debug/release |
| `DB_TYPE` | `sqlite` | 数据库类型: sqlite/mysql/postgres |
| `DB_PATH` | `./data/newmcp.db` | SQLite 数据库路径 |
| `SQL_DSN` | - | MySQL/PostgreSQL DSN |
| `REDIS_CONN_STRING` | - | Redis 连接字符串 (可选) |
| `SESSION_SECRET` | - | Session 密钥 (多实例必须设置) |
| `CRYPTO_SECRET` | - | 加密密钥 (加密 API Key 等敏感数据) |
| `LOG_LEVEL` | `info` | 日志级别: debug/info/warn/error |

> [!NOTE]
> MCP 端点地址由**系统设置**中的**服务器地址**（`ServerAddress`）决定，管理员可在后台运行时动态修改，无需配置环境变量。

---

## 3. 反向代理配置

### 3.1 Nginx

```nginx
server {
    listen 80;
    server_name newmcp.example.com;

    # WebSocket 支持
    location /mcp/ws {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }

    # SSE 支持 (Streamable HTTP)
    location /mcp/ {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding on;
        proxy_read_timeout 3600s;
    }

    # REST API 和静态文件
    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## 4. MCP 客户端连接配置

### 4.1 Claude Code

编辑 `~/.claude/claude_desktop_config.json` 或项目 `.claude/settings.json`:

```json
{
    "mcpServers": {
        "newmcp-robot": {
            "type": "streamable-http",
            "url": "http://localhost:3000/mcp/group/robot",
            "headers": {
                "X-API-Key": "sk-your-api-key"
            }
        }
    }
}
```

### 4.2 Cursor

编辑 `.cursor/mcp.json`:

```json
{
    "mcpServers": {
        "newmcp-all": {
            "type": "streamable-http",
            "url": "http://localhost:3000/mcp",
            "headers": {
                "X-API-Key": "sk-your-api-key"
            }
        }
    }
}
```

### 4.3 云端主动连接 (小智等平台)

在 NewMCP 管理界面 → 云端连接 → 添加连接，选择云平台类型（如小智），粘贴远端平台提供的 WSS URL 即可。

### 4.4 通用 MCP 客户端 (stdio 桥接)

对于仅支持 stdio 的客户端，使用 `mcp-proxy` 桥接:

```bash
npx mcp-proxy --transport streamablehttp http://localhost:3000/mcp/group/robot
```

---

## 5. 数据备份

### SQLite
```bash
# 备份
sqlite3 /data/newmcp.db ".backup /data/newmcp_backup.db"

# 恢复
cp /data/newmcp_backup.db /data/newmcp.db
```

### MySQL
```bash
# 备份
docker exec newmcp-mysql mysqldump -u newmcp -pnewmcp newmcp > backup.sql

# 恢复
docker exec -i newmcp-mysql mysql -u newmcp -pnewmcp newmcp < backup.sql
```
