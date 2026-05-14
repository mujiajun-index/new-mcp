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
# Copy frontend build output into web/dist so it's embedded at runtime
COPY --from=frontend-builder /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -o /newmcp ./cmd/server/

# ---- Stage 3: Runtime ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=backend-builder /newmcp /app/newmcp
COPY --from=backend-builder /app/web/dist /app/web/dist

ENV PORT=3000
ENV GIN_MODE=release
ENV DB_TYPE=sqlite
ENV DB_PATH=/app/data/newmcp.db

EXPOSE 3000

VOLUME ["/app/data"]

ENTRYPOINT ["/app/newmcp"]
