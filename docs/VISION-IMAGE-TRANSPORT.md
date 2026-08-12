# 视觉工具图片传参架构重构（base64 → URL 透传）

> 版本: V1.0 | 状态: 已实现 | 更新日期: 2026-08-12
> 关联文档: [ARCHITECTURE.md](./ARCHITECTURE.md) · [API.md](./API.md) · [DATABASE.md](./DATABASE.md) · [MCP-PROTOCOL.md](./MCP-PROTOCOL.md)
>
> **变更摘要**:
> - 视觉工具图片传参从 **base64 全量内联** 重构为 **上传到存储 → 短时签名 URL → 上游 provider 自己拉图**，图片字节绕开调用方 LLM 上下文。
> - 新增 `Storage` 抽象（local 磁盘 / S3 兼容双后端）、multipart 上传端点（JWT + API Key 双鉴权）、HMAC 签名取文件端点、`ImageInput` 可辨识联合、过期清理后台任务。
> - 保留 base64 入参向后兼容，并加 `VisionUploadMaxBytes` 上限。
> - 设计参照智谱 `@z_ai/mcp-server` 视觉 MCP，适配 new-mcp 的远程 HTTP 网关形态。

---

## 1. 背景与动机

### 1.1 旧链路的问题

视觉工具（`analyze_image` / `describe_scene`）的 `image` 入参是一段纯 base64 字符串，整条链路是「base64 进 → 解码嗅探 → 重编码 → base64 内联转发上游」，全程无大小上限、无压缩、无 URL 化。

真正的痛点不是网关内存，而是 **base64 作为 MCP 工具参数文本进入了调用方 LLM 的上下文**：一张 1–2MB 的图 ≈ 50 万 token（对比：同一张图作为视觉输入仅 ~1–1.6k tokens）。调用方 LLM（Claude Code 等）得把整段 base64 当 **output token** 生成出来，撑爆上下文、成本爆炸，大图直接不可用。

### 1.2 业界标准解法

智谱 `@z_ai/mcp-server`、BlackOps、MCP 官方 Working Group 的共识是让字节绕开 LLM 上下文：

- **图片先上传到存储** → 拿短时签名 URL → 工具只传 URL → 上游 provider 自己拉图。
- 智谱按字节来源拆工具（`file_paths` 走本地路径、`urls` 走公网透传），视频/文件强制只走 URL。

new-mcp 是远程 HTTP 网关，读不到客户端本地盘，故把智谱的「本地路径」模式替换为「**上传到网关存储 → 签名 URL**」——对等、且对调用方无感。

### 1.3 预期结果

| 维度 | 旧（base64 全内联） | 新（URL 透传） |
|------|--------------------|----------------|
| 调用方 LLM 上下文占用 | 整张图 base64 ≈ 50 万 token / 1-2MB | 一个短 URL，几十字节 |
| 网关内存 | 全量解码 + 重编码 | 上传路径有界读取；分析路径零下载 |
| 大图可用性 | 基本不可用 | 受 `VisionUploadMaxBytes` 上限约束，默认 10MB |
| SSRF 风险 | N/A | 无：网关永不下载 `image_url`，纯透传给上游 |
| 向后兼容 | — | base64 入参保留，仅加上限 |

---

## 2. 架构总览

```
上传:  调用方 --multipart--> /api/v1/vision/upload(JWT)        -->
         --> sniffMediaType(magic byte) + sha256 --> Storage.Put --> 写 uploaded_images 元数据
         --> 返回 {url: Storage.PublicURL(key, ttl), expires_at, mime, size, key, backend, deduped}
       同一路径另一鉴权：/api/v1/vision/mcp-upload(APIKey+RateLimit)，共享同一 handler

分析:  LLM 调 vision tool {image_url: <签名URL 或 任意外部 https URL>}
         --> vision_handler 仅校验 https + host --> ImageInput{URL} --> Analyze 透传给上游
         --> 上游自己 GET 该 URL（local 后端时命中公开的 /api/v1/vision/files/*key 端点；s3 后端时直连桶 presign URL）
兼容:  {image: <base64>} --> DecodeImage + 大小上限 --> ImageInput{Bytes} --> 客户端按 provider 编码内联
camera: 本地 JPEG 字节 --> ImageInput{Bytes, "image/jpeg"} --> 不变
清理:  后台 ticker 按 created_at < now - UploadRetentionHours 删行 + Storage.Delete 对象
```

核心不变量：**new-mcp 永不下载 `image_url`**。URL 仅做 `https` + 非空 host 校验后透传给上游，因此没有 SSRF 面；图片字节在上传时一次性写入存储，分析阶段既不进网关内存也不进调用方 LLM 上下文。

---

## 3. 新增 / 修改文件

### 3.1 新增文件

| 文件 | 职责 |
|------|------|
| `internal/storage/storage.go` | `Storage` interface + `Config` + `New(ctx,cfg)` 工厂 + `ContentKey` + `LoadConfig`（按 `model.GetOption*` 选后端）+ `ErrObjectNotFound` 哨兵 |
| `internal/storage/local.go` | 磁盘实现：`Put`/`Get`/`Delete` + `PublicURL`（自签 HMAC URL 指回 new-mcp 自有取文件端点）+ 路径遍历防护 |
| `internal/storage/s3.go` | minio-go v7 实现：`Put`/`Get`/`Delete` + `PublicURL`（桶 presigned GET）+ 启动期 `BucketExists` 探活 + `NoSuchKey` 映射到 `ErrObjectNotFound` |
| `internal/storage/sign.go` | HMAC-SHA256：`SignURL(key,expires)` / `VerifyURL(key,expires,token)`（`crypto/subtle` 常量时间比对）+ `IsSecretConfigured`；密钥用 `common.SessionSecret` |
| `internal/storage/local_test.go` | 单测：Put/Get 往返、缺失文件返回哨兵、Delete 幂等、路径遍历拒绝、签名/校验、`PublicURL` 形状与签名一致性 |
| `model/uploaded_image.go` | `UploadedImage` GORM 模型 + `Insert`/`Update`/`GetByKey`/`TouchRefresh`/`ListExpiredUploads`/`DeleteByID` |
| `service/upload.go` | `UploadService.Upload`：有界读字节 → magic-byte 嗅探 → sha256 内容寻址 key → 去重（命中则跳过 Put + `TouchRefresh`）→ `Storage.Put` → 写元数据 → 返回签名 URL |
| `service/upload_cleanup.go` | `StartCleanupLoop(ctx)` + `runCleanupSweep` + `cleanupInterval` / `uploadRetention`（均按 option 动态读） |
| `service/upload_cleanup_test.go` | 单测：过期清理按时序 + 动态保留期调小下次 sweep 生效 |
| `controller/upload.go` | `UploadVisionImage`（multipart POST，双路由共享）+ `GetVisionFile`（公开签名 URL GET，流式输出） |
| `controller/upload_integration_test.go` | 端到端：上传 → serve 还原 → 去重 → 篡改 token 403 → 过期 410 |
| `internal/mcp/vision/media.go` | `SniffMediaType`（从 vision_handler 提升出来共享，magic-byte 嗅探 jpeg/png/gif/webp） |
| `router/router_smoke_test.go` | 单测：注册 API 路由不 panic（防 gin radix-tree 冲突，如 `/vision/upload` vs `/vision/:id/enable`） |

### 3.2 修改文件

| 文件 | 改动 |
|------|------|
| `internal/mcp/vision/client.go` | `Analyze` 改签名收 `ImageInput`；OpenAI / Anthropic / Gemini 三家加 URL 分支；新增 `anthropicSource.URL`、`geminiPart.FileData` / `geminiFileData`；base64 编码下沉进各 provider 分支 |
| `internal/mcp/virtual/vision_handler.go` | 入参 struct 加 `ImageURL`；`image`/`image_url` 二选一分派；https 校验 + base64 大小上限；删除重复的 `sniffMediaType`（改用 `vision.SniffMediaType`） |
| `internal/mcp/virtual/camera_handler.go` | 调用点改 `vision.ImageInput{Bytes: frame, MediaType: "image/jpeg"}`，逻辑不变 |
| `service/vision.go` | `TestVision` 调用点改 `ImageInput`（base64 先 decode 成字节）；`buildToolsCache` 两工具加 `image_url`、去掉 `image` 的 `required`（handler 强制二选一） |
| `router/api_router.go` | auth 组加 `POST /vision/upload`；公开组加 `GET /vision/files/*key` + `POST /vision/mcp-upload`（APIKeyAuth + RateLimit） |
| `router/main.go` | `InitGateway` 里构 Storage 单例存 `service.UploadStore`（`initUploadStorage`，附启动告警）；新增 `StartBackgroundJobs` / `StopBackgroundJobs` |
| `cmd/server/main.go` | `srvCtx` 创建后调 `router.StartBackgroundJobs(srvCtx)`，`defer StopBackgroundJobs()` |
| `model/option.go` | `defaultOptions` 加存储/上传配置键；`sensitiveKeys` 加 `StorageAccessKey` / `StorageSecretKey` |
| `model/main.go` | `migrateDB` AutoMigrate 列表加 `&UploadedImage{}` |
| `go.mod` | `go get github.com/minio/minio-go/v7`（v7.2.1） |

---

## 4. Storage 抽象

### 4.1 接口定义

```go
type Storage interface {
    Put(ctx, key string, r io.Reader, mimeType string) error
    Get(ctx, key string) (io.ReadCloser, error)        // local 自 serve 用；s3 供 GetObject
    Delete(ctx, key string) error                       // 幂等，清理任务用
    PublicURL(ctx, key string, ttl time.Duration) (string, error) // local=自签URL, s3=PresignedGetObject
    Backend() string                                     // "local"/"s3"
}
```

local 与 s3 的差异**全部藏在 `PublicURL` 后面**：local 写盘 + 返回指回 new-mcp 的 HMAC URL；s3 写桶 + 返回桶 presign URL。上传 / 分析代码不分支。

### 4.2 Key 策略（内容寻址，天然去重）

- `ContentKey(sha256hex)` = `<sha[:2]>/<sha>`，如 `ab/abcdef...`。两级 shard 避免单目录 / 单桶前缀无限膨胀。
- 上传时已算 sha256，key 免费；相同字节 → 相同 key → `GetUploadedImageByKey` 命中则跳过 `Put`、仅 `TouchRefresh` 续期。
- 并发同字节上传的兜底：loser 在 `Insert` 唯一键冲突时回收冗余 blob，复用 winner 的行。

### 4.3 凭证缺失行为

| 后端 | 缺凭证 | 行为 |
|------|--------|------|
| `local`（默认） | — | 零凭证，`os.MkdirAll` 建根目录 |
| `s3` | 缺 endpoint/bucket/access/secret 任一 | **启动直接报错**（显式配置错误，拒绝静默回退到 local） |

### 4.4 签名 URL（local 后端）

格式：

```
{ServerAddress}/api/v1/vision/files/{key}?expires={unixSec}&token={hex(HMAC-SHA256(SessionSecret, key+"|"+expires))}
```

- `{ServerAddress}` = `strings.TrimRight(model.GetOptionString("ServerAddress"), "/")`，每次调用现读，管理员改完下次上传即生效。
- 取文件端点挂**公开组**（签名即鉴权，不套 UserAuth/APIKeyAuth——上游 provider 无 JWT/sk 头）。
- 校验：`expires` 过期 → 410；`crypto/subtle.ConstantTimeCompare` 比对 HMAC → 不匹配 403；`UploadedImage.GetByKey` 拒已清理行 → 404；`Storage.Get` → `c.DataFromReader` 流式输出，带 `Content-Type` / `Cache-Control: private, no-store` / `X-Content-Type-Options: nosniff`。
- HMAC 密钥用 `common.SessionSecret`，与 JWT 共用密钥生命周期（`SESSION_SECRET` 轮换同时失效签名 URL 与 JWT）。未设时回退弱默认 `"default-secret-change-me"` 并启动告警。
- TTL 默认 3600s；防篡改靠 HMAC，防重放接受 TTL 内（同 S3 presign 模型）。

### 4.5 S3 后端

- minio-go v7 覆盖 AWS S3 / MinIO / Cloudflare R2 / B2 / Alibaba OSS / Tencent COS。
- `PublicURL` 返回桶 `PresignedGetObject` URL——上游 provider 直连桶，new-mcp 不在数据路径上。
- 启动期 `BucketExists` 探活，桶不存在即报错；`NoSuchKey`/`NoSuchObject` 映射到 `ErrObjectNotFound` 进入幂等删除路径。

---

## 5. ImageInput 可辨识联合

### 5.1 类型

```go
type ImageInput struct {
    Bytes     []byte
    MediaType string  // Bytes 路径必填；URL 路径可空
    URL       string
}
func (in ImageInput) IsURL() bool { return in.URL != "" }
```

`Analyze(ctx, systemPrompt, userPrompt string, in ImageInput)` 按 provider 透传或内联：

| Provider | URL 路径 | Bytes 路径 |
|----------|----------|------------|
| OpenAI | `image_url.url` 直接填 URL | 拼 data URL：`"data:"+mediaType+";base64,"+...` |
| Anthropic | `source.type="url"`，`source.url` 填 URL | `source.type="base64"`，`source.media_type` + `source.data` |
| Gemini | `file_data.file_uri` 填 URL | `inline_data.mime_type` + `inline_data.data` |

三个调用点（`vision_handler`、`camera_handler`、`service/vision.go::TestVision`）全改 `ImageInput`，语义不变。

> ⚠️ **Gemini 原生 `file_uri` 对任意 https URL 可能不稳**：其原生面向自家 Files API / GCS URI。V1 的可靠 URL 路径是 **OpenAI 兼容与 Anthropic**；Gemini 分支已实现为完整性保留。

---

## 6. vision_handler 分派逻辑

```go
var input vision.ImageInput
switch {
case params.ImageURL != "":
    u, err := url.Parse(params.ImageURL)
    if err != nil || u.Scheme != "https" || u.Host == "" {
        return nil, fmt.Errorf("image_url must be an https URL")
    }
    input.URL = params.ImageURL
case params.Image != "":
    imgBytes, mediaType, err := DecodeImage(params.Image)  // 复用既有解码
    if err != nil { return nil, fmt.Errorf("invalid image: %w", err) }
    if max := model.GetOptionInt64("VisionUploadMaxBytes"); max > 0 && int64(len(imgBytes)) > max {
        return nil, fmt.Errorf("image exceeds the %d-byte limit", max)
    }
    input.Bytes, input.MediaType = imgBytes, mediaType
default:
    return nil, fmt.Errorf("either image or image_url is required")
}
```

工具 schema（`buildToolsCache`）两工具均加 `image_url`、**去掉 `image` 的 `required`**——handler 强制二选一，schema 不再单锁一个。

---

## 7. 上传与取文件 API

### 7.1 上传端点（双鉴权，同一 handler）

| 路由 | 中间件 | 用途 |
|------|--------|------|
| `POST /api/v1/vision/upload` | `UserAuth`（JWT） | Web UI |
| `POST /api/v1/vision/mcp-upload` | `APIKeyAuth` + `RateLimit` | MCP 调用方（sk-） |

> gin 的 radix-tree 不允许同路径挂两套中间件，故分两条路径共享 `controller.UploadVisionImage`。两套中间件向 context 注入 user id 的 key 不同：`UserAuth` 注入 `user_id`，`APIKeyAuth` 注入 `api_key_user_id`——handler 先读 `user_id`，为 0 时回退 `api_key_user_id`。

**请求**：`multipart/form-data`，字段 `image` 为单张图片。

**响应** `201 Created`：

```json
{
  "success": true,
  "message": "created",
  "data": {
    "url": "http://host/api/v1/vision/files/ab/abcd...?expires=...&token=...",
    "expires_at": "2026-08-12T17:00:00Z",
    "mime": "image/png",
    "size": 12345,
    "key": "ab/abcd...",
    "backend": "local",
    "deduped": false
  }
}
```

调用方把 `url` 作为 `image_url` 传给 vision 工具即可。

### 7.2 取文件端点（公开）

| 路由 | 中间件 | 说明 |
|------|--------|------|
| `GET /api/v1/vision/files/*key` | 无（签名即鉴权） | catch-all `*key` 容纳 `ab/<sha>` 多段 |

响应：流式返回原图字节，`Content-Type` = 上传时嗅探的 mime；`Cache-Control: private, no-store`；`X-Content-Type-Options: nosniff`。

| 失败条件 | 状态码 |
|----------|--------|
| 缺 / 非法 `expires` 或缺 `token` | 403 |
| `expires` 已过期 | 410 |
| HMAC 不匹配（含篡改） | 403 |
| key 不在 `uploaded_images`（已清理 / typo） | 404 |
| 对象已从存储消失（TOCTOU） | 404 |

### 7.3 curl 端到端示例

```bash
# 1) 上传（Web UI / JWT）
curl -F image=@photo.jpg -H "Authorization: Bearer <jwt>" \
  http://localhost:3000/api/v1/vision/upload
# => {"data":{"url":"http://localhost:3000/api/v1/vision/files/ab/...?expires=...&token=...", ...}}

# 2) 上传（MCP 客户端 / API Key）
curl -F image=@photo.jpg -H "X-API-Key: sk-..." \
  http://localhost:3000/api/v1/vision/mcp-upload

# 3) 取文件（公开，签名即鉴权；上游 provider 也这样拉图）
curl "<url>"

# 4) 用作 image_url 调 vision 工具（JSON-RPC tools/call，节选）
curl -X POST http://localhost:3000/mcp/group/<slug> -H "X-API-Key: sk-..." -d '{
  "method":"tools/call",
  "params":{"name":"vision.analyze_image","arguments":{"image_url":"<url>","prompt":"描述这张图"}}
}'
```

---

## 8. 数据模型

`uploaded_images` 表（`model/uploaded_image.go`）：

| 列 | 类型 | 说明 |
|----|------|------|
| `id` | int64 PK | |
| `user_id` | int64, index | 上传者（JWT 或 API Key 归属用户） |
| `storage_key` | varchar(128), unique index | 内容寻址 key `ab/<sha256>` |
| `media_type` | varchar(64) | magic-byte 嗅探的 mime |
| `size` | int64 | 解码后字节长度 |
| `backend` | varchar(16) | `local` / `s3`——blob 所在后端 |
| `created_at` | time, index | **保留期时钟**：`< now - UploadRetentionHours` 即被清理 |
| `updated_at` | time | |

> ⚠️ **无 `gorm.DeletedAt`**：行是短生命周期，清理任务硬删行 + 对象。软删墓碑会破坏内容寻址去重（同字节重传会建重复行而不是复活已有 key）。

**保留期是动态的**——没有 `expires_at` 列。清理 sweep 每次 `now - UploadRetentionHours` 重算 cutoff，所以管理员**调小 `UploadRetentionHours` 在下次 tick 立即生效**，已有上传也会被回收。同字节重传走 `TouchRefresh`（重置 `created_at`）续期，热门图按最近一次请求计寿命。

---

## 9. 配置键

`model/option.go` `defaultOptions` 新增（`options` 表已存在，无需 migration）：

| 键 | 默认 | 说明 |
|----|------|------|
| `StorageBackend` | `local` | `local` \| `s3` |
| `StorageLocalPath` | `./data/uploads` | local 磁盘根目录 |
| `StoragePathPrefix` | `vision` | 磁盘 / 桶内物理隔离前缀 |
| `StorageEndpoint` | （空） | s3 兼容端点 |
| `StorageRegion` | （空） | s3 region |
| `StorageBucket` | （空） | s3 bucket 名 |
| `StorageAccessKey` | （空） | s3 access key（**sensitive**） |
| `StorageSecretKey` | （空） | s3 secret key（**sensitive**） |
| `StorageUseSSL` | `true` | s3 端点是否走 https |
| `VisionUploadMaxBytes` | `10485760` | 单图字节上限（解码后），10MB |
| `SignedURLTTLSeconds` | `3600` | 签名 URL 有效期，1h |
| `UploadRetentionHours` | `24` | 上传保留时长，须 > `SignedURLTTLSeconds` |
| `UploadCleanupIntervalMinutes` | `60` | 过期清理扫描间隔 |

`StorageAccessKey` / `StorageSecretKey` 已加入 `sensitiveKeys`，管理员设置接口不会回显值。

---

## 10. 后台清理任务

`service/upload_cleanup.go::StartCleanupLoop(ctx)`：

- ticker 按 `UploadCleanupIntervalMinutes` 周期触发；`ctx` 取消即退出（`router.StopBackgroundJobs` → `backgroundJobsCancel`）。
- 每次 sweep 取 `ListExpiredUploads(now - retention, 500)`（有界，防大批积压单 tick 占满），逐项 `Storage.Delete` + `DeleteUploadedImageByID`；单项失败只 log 不中止整轮（行仍删除，残留 blob 无害 / 下次幂等回收）。

接线：`cmd/server/main.go` 在 `srvCtx` 创建后调 `router.StartBackgroundJobs(srvCtx)`，`defer router.StopBackgroundJobs()`，与 `StartCloudConnections` / `StopCloudConnections` 对称。

---

## 11. 安全模型

| 威胁 | 处置 |
|------|------|
| **SSRF** | new-mcp **永不下载** `image_url`，纯透传给上游；vision_handler 仅校验 `scheme==https && host!=""`，无出站拉取 |
| **文件类型欺骗** | 上传与 serve 都走 `SniffMediaType`（magic byte），不信扩展名；空 mime 拒绝 |
| **上传大小** | `http.MaxBytesReader`（controller，`FormFile` 前）+ base64 解码后 `len` 上限（vision_handler），双重强制 |
| **路径遍历**（local） | `fullpath` 清洗 key，拒 `..` / 反斜杠 / 空段 / `//`，校验 `filepath.Join(root,key)` 仍在 root 下 |
| **签名 URL 伪造** | HMAC-SHA256，`crypto/subtle` 常量时间比对；密钥 = `SESSION_SECRET`，缺则弱默认 + 启动告警 |
| **签名 URL 重放** | TTL 内接受（同 S3 presign 模型），`expires` 过期 → 410 |
| **SessionSecret 为空** | `initUploadStorage` 启动告警（避免签名 URL 用默认弱密钥）；生产应设 `SESSION_SECRET` |
| **ServerAddress 未改默认值** | local 后端 + `localhost` / `127.0.0.1` 时启动告警（上游拉不到图） |
| **清理 / 使用冲突** | 保留期(24h) > 签名 TTL(1h)，TTL 先过期上游已拉到字节，删除在后；TOCTOU 窗口可接受（上游可重试） |

---

## 12. 验证

### 12.1 自动化测试

| 包 | 测试 | 覆盖 |
|----|------|------|
| `internal/storage` | `local_test.go` | Put/Get 往返、缺失返回哨兵、Delete 幂等、路径遍历拒绝、Sign/Verify 全分支、`PublicURL` 形状与签名一致性 |
| `router` | `router_smoke_test.go` | 注册全部 API 路由不 panic（防 gin radix-tree 冲突） |
| `controller` | `upload_integration_test.go` | 上传 → serve 还原字节 → 同字节去重 → 篡改 token 403 → 过期 410 |
| `service` | `upload_cleanup_test.go` | 过期清理按时序 + 动态保留期调小下次 sweep 立即生效 |

`go build ./...`、`go vet ./...` 均绿。

### 12.2 端到端（手动，需运行实例）

```bash
go build ./... && ./newmcp  # 启动后 uploaded_images 表自动建成

# 1) local 路径：JWT 上传 → 拿 url → curl 取回原图
# 2) MCP URL 路径：把 url 作为 image_url 调 vision.analyze_image → 看上游日志里是 URL 而非 base64
# 3) base64 回归：仍可用 image(base64) 调通；超 VisionUploadMaxBytes 被拒
# 4) camera 回归：camera.analyze 正常
# 5) MCP 调用方上传：sk- 鉴权 /vision/mcp-upload 通
# 6) S3 路径：配 StorageBackend=s3 + 凭证，重复 2-3，对象入桶、URL 指向桶
# 7) 清理：调小 UploadRetentionHours，等一个 tick，旧行 + 对象被删，近期保留
# 8) 安全：过期 URL→410；篡改 token→403；非 https image_url→工具报错
```

---

## 13. 范围与待办

### 13.1 本次交付（后端全量）

- `Storage` 接口 + **local 默认实现**（零依赖）+ **S3 兼容实现**（minio-go v7）
- **双鉴权上传端点**（JWT + API Key）+ **公开签名 URL 取文件端点**
- `ImageInput` 重构 + 三家 provider URL 透传分支
- vision 工具 `image_url` 参数 + 二选一分派 + base64 上限
- 过期清理后台任务 + 动态保留期
- 全量单测 + 端到端集成测试 + 构建全绿

### 13.2 待办 / 未覆盖

| 项 | 说明 |
|----|------|
| **S3 真实桶端到端** | s3 后端已 build / vet / 单测，但未对真实桶跑上传 / 拉图验证；需配 `StorageBackend=s3` + 凭证执行 §12.2 第 6 步 |
| **存量 vision 配置重新同步** | 旧 vision 配置的 `ToolsCache` 仍是旧 schema（无 `image_url`）；**需重新保存 / 启用**才 regenerate 暴露 `image_url`。新建配置直接生效 |
| **前端未做** | 管理后台存储配置 UI、vision 测试面板的上传按钮未做——settings 的 key / value 接口 + curl / MCP 客户端已可跑通全流程，前端可后置 |
| **Gemini 原生 file_uri** | 对任意 https URL 可能不稳（V1 可靠 URL 路径为 OpenAI 兼容与 Anthropic） |
| **网关侧 resize**（可选降本） | 未做；OpenAI / Claude 上游本就会做长边 ≤1568px 的 resize，非阻塞 |
| **StoreAddress 反向回退**（plan 遗留钩子） | 当 `image_url` 主机 == 自家 `ServerAddress` 主机时回退 `ImageInput{Bytes}`（无 SSRF，因是自家文件），可作 Gemini 路径兜底，未实现 |

### 13.3 协议趋势（短期不依赖）

MCP WG 的 `SEP-2532`（resources/stream）、`SEP-2166`（httpUrl/httpUrlExpiresAt）、issue #527 均为 Draft。截至 2026-07-28，`resources/read` 与 tool result image 仍只支持 base64——本次方案正是绕开协议未定案的过渡工程实现。