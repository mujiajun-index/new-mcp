# ---- Stage 1: Build frontend ----
FROM node:20-alpine AS frontend-builder

WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- Stage 2: Build backend ----
FROM golang:1.26-alpine AS backend-builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy frontend build output so //go:embed web/dist embeds it into the binary
COPY --from=frontend-builder /app/web/dist ./web/dist
# Inject version from VERSION file via ldflags (default v0.0.0 if not injected)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w -X 'github.com/mujkjk/newmcp/common.Version=$(cat VERSION)'" -o /newmcp ./cmd/server/

# ---- Stage 3: Runtime ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=backend-builder /newmcp /app/newmcp

ENV PORT=3000
ENV GIN_MODE=release
ENV DB_TYPE=sqlite
ENV DB_PATH=/app/data/newmcp.db

EXPOSE 3000

VOLUME ["/app/data"]

ENTRYPOINT ["/app/newmcp"]
