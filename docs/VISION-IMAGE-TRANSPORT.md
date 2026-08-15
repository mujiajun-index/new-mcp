# 视觉工具图片传参架构重构（base64 → URL 透传）

> 版本: V1.3 | 状态: V1.0 已实现 · V1.1 已实现（后端 + 前端管理 UI）· V1.2 已实现（`upload_image` 移入视觉 MCP）· V1.3 已实现（自家 URL 反向取字节） | 更新日期: 2026-08-15
> 关联文档: [ARCHITECTURE.md](./ARCHITECTURE.md) · [API.md](./API.md) · [DATABASE.md](./DATABASE.md) · [MCP-PROTOCOL.md](./MCP-PROTOCOL.md)
>
> **变更摘要**:
> - 视觉工具图片传参从 **base64 全量内联** 重构为 **上传到存储 → 短时签名 URL → 上游 provider 自己拉图**，图片字节绕开调用方 LLM 上下文。
> - 新增 `Storage` 抽象（local 磁盘 / S3 兼容双后端）、multipart 上传端点（JWT + API Key 双鉴权）、HMAC 签名取文件端点、`ImageInput` 可辨识联合、过期清理后台任务。
> - 保留 base64 入参向后兼容，并加 `VisionUploadMaxBytes` 上限。
> - 设计参照智谱 `@z_ai/mcp-server` 视觉 MCP，适配 new-mcp 的远程 HTTP 网关形态。
>
> **V1.1 变更（shell 命令行直传 + 图片管理 + 入参择优，已实现，详见 §14、§15、§16）**:
> - 新增 `upload_image` 工具：传入 `local_path`，返回**预签名 PUT 的现成 curl 命令** + `image_url`；模型用 Bash 执行 curl 把图片直传存储，**curl 命令里不含任何 API Key**（预签名 URL 本身就是凭证）。
> - 解决 V1.0 multipart 路径在「模型自己用 curl 上传」场景下的 key 暴露问题（`Authorization: Bearer sk-...` 会进模型上下文）。
> - 两后端（local / s3）藏在同一 `Storage.PutURL` 抽象后，模型侧 curl 形态一致；新增 `PUT /api/v1/vision/files/*key` 直传端点、purpose 绑定的 HMAC 签名、`uploaded_images.status` 状态机。
> - **新增图片管理**（§15）：用户 / 管理员列表 + 删除 API、`pending` 行快清、`MaxUploadsPerUser` 护栏；上传仍不计费（`billing/` 只管工具调用）。
> - **`analyze_image` 入参择优**（§16）：小图（≤ `VisionInlineMaxBytes`，默认 10KB）直接 base64 内联、大图走 `upload_image`；按尺寸分档，避免小图也强制上传、大图撑爆上下文。
>
> **V1.2 变更（`upload_image` 由全局工具改为 per-config 内置工具，详见 §14.2）**:
> - `upload_image` 不再作为全局虚拟工具（`global.go`）对所有人常驻追加；改为**每个视觉服务（`vision_<id>`）的第三个内置工具**，名称 `vision.upload_image` 与描述**固定不可改**（不进 `VisionConfig` 字段），随 `buildToolsCache` 注入、经 per-user `VirtualToolRegistry` 派发（暴露名 `vision_<id>__vision.upload_image`，与 `analyze_image` 同路径）。
> - 删除 `internal/mcp/virtual/global.go`（`GlobalTools`/`IsGlobalTool`/`HandleGlobalTool`）及 `gateway_handler` 的两处全局短路；调用方 userID 经 context（`virtual.WithCallerUserID`/`CallerUserID`）在两处 virtual 派发点注入，`VisionHandler` 的 upload 分支读取（回退 `vc.UserID`）。
> - 视觉配置详情页新增只读 `upload_image` 卡片（标题/简介/「系统内置·不可修改」徽标）。存量已启用配置需重新保存/启用以 regenerate `ToolsCache`（沿用 `image_url` 的既定约定）；新配置直接生效。
> - `analyze_image`/`describe_scene` 的 `image_url` 入参**放开为 http 与 https 均可**（原先仅 https）。网关永不下载 `image_url`（纯透传上游），scheme 不构成 SSRF 面；放行 http 兼容 `ServerAddress` 为 http 的本地/非 TLS 部署（如默认 `http://localhost:3000`）。
>
> **V1.3 变更（自家存储 URL 反向取字节，详见 §14.7）**:
> - `image_url` 若被 `OwnsURL` 判定为**自家存储 URL**（`ServerAddress` 同主机的签名 GET URL，或 S3 presigned GET URL），网关**改读自家对象字节并以 base64 直接发上游**（`ImageInput{Bytes}`，与 camera 路径一致），不再透传 URL。任意外部 http(s) URL 仍是纯透传（网关**永不**下载外部 URL，无 SSRF）。
> - 动机：本地部署时上游 provider 拉不到 `ServerAddress`（localhost / 内网不可达），原「上游自己 GET」路径必败；Gemini 原生 `file_uri` 对任意 URL 也不稳。自家字节经 `OwnsURL` 校验是网关自己签发的，读它无 SSRF 面；调用方 LLM 仍只持有短 `image_url`，字节不进其上下文。
> - `Storage` 接口加 `KeyFromURL(rawurl) (key, ok)`（local 从 `/api/v1/vision/files/<key>` 路径剥 key；s3 从 presigned URL 路径定位 `/<pathPrefix>/` 取后缀）；`vision_handler` 的自家 URL 分支由「Stat 确认 + 透传」改为「`KeyFromURL` → `Stat` 大小校验 → `Get` → `SniffMediaType` → `ImageInput{Bytes}`」，原 `ownStorageKeyFromURL` 自由函数删除。`VisionInlineMaxBytes` 不适用（字节走 new-mcp→上游，不进调用方 LLM 上下文）；`VisionUploadMaxBytes` 仍是硬上限（Get 前按 `Stat` 拒大）。
>
> **提示词对齐微调（2026-08-15）**:
> - `upload_image` 返回文本中的 `file_url` 字段改名 `image_url`，与 `analyze_image`/`describe_scene` 的入参**同名**；全部提示词（`upload_image` 工具 description、工具返回文本、`initialize` instructions、`image_url` 参数 description）中的 `image_url=file_url` 映射写法删除。返回值与目标参数同名，模型可**原样透传**，消除弱模型把字面量 `file_url` 当值传、或映射改错名的失败模式。

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
| SSRF 风险 | N/A | 无：外部 `image_url` 永不被下载（纯透传上游）；自家存储 URL 经 `OwnsURL` 校验为网关自己签发，读本地对象无 SSRF 面 |
| 向后兼容 | — | base64 入参保留，仅加上限 |

---

## 2. 架构总览

```
上传:  调用方 --multipart--> /api/v1/vision/upload(JWT)        -->
         --> sniffMediaType(magic byte) + sha256 --> Storage.Put --> 写 uploaded_images 元数据
         --> 返回 {url: Storage.PublicURL(key, ttl), expires_at, mime, size, key, backend, deduped}
       同一路径另一鉴权：/api/v1/vision/mcp-upload(APIKey+RateLimit)，共享同一 handler

直传(V1.1): 模型调 upload_image(local_path) --> 网关生成 uuid key + 预签名 PUT URL + 签名 GET URL
         --> 返回 {upload_command: "curl(.exe) -X PUT -T <path> '<put_url>'"(按 OS 推断二进制名,无 key) + image_url}  # 见 §14.2.1 跨系统命令生成
         --> 模型用 Bash 跑 upload_command：s3 后端直传桶(字节绕过网关) / local 后端命中 PUT /api/v1/vision/files/*key(字节经网关但无 key)
         --> 模型调 analyze_image(image_url=返回的 image_url) --> vision_handler 判 OwnsURL 自家 URL
         --> KeyFromURL 剥 key → Stat 大小校验 → Get 读字节 → SniffMediaType → ImageInput{Bytes} 发上游
         --> 上游直接收 base64（无需回访 ServerAddress，本地部署也能跑）

分析:  LLM 调 vision tool {image_url: <签名URL 或 任意外部 http(s) URL>}
         --> vision_handler 仅校验 http(s) + host
         --> 自家 URL: ImageInput{Bytes}（反向取字节，V1.3）；外部 URL: ImageInput{URL}（透传，上游自己 GET）
         --> camera / base64 入参: ImageInput{Bytes} --> 客户端按 provider 编码内联
兼容:  {image: <base64>} --> DecodeImage + 大小上限 --> ImageInput{Bytes} --> 客户端按 provider 编码内联
camera: 本地 JPEG 字节 --> ImageInput{Bytes, "image/jpeg"} --> 不变
清理:  后台 ticker 按 created_at < now - UploadRetentionHours 删行 + Storage.Delete 对象
```

核心不变量：**new-mcp 永不下载外部 `image_url`**。外部 URL 仅做 `http(s)` + 非空 host 校验后透传给上游，因此没有 SSRF 面。自家存储 URL（经 `OwnsURL` 判定为网关自己签发）则反向读取本地对象字节并以 base64 发上游——读的是自家对象，同样无 SSRF 面。无论哪条路径，图片字节在分析阶段都不进调用方 LLM 上下文（调用方只持有短 `image_url`）。

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
    Get(ctx, key string) (io.ReadCloser, error) // /u/<sid> 端点 + 反向取图；local 直读盘、s3 供 GetObject
    Delete(ctx, key string) error                // 幂等，清理任务用
    Stat(ctx, key string) (ObjectInfo, error)    // 反向取图前按大小校验 VisionUploadMaxBytes
    Backend() string                             // "local"/"s3"
}
```

local 与 s3 的差异**全部藏在 `Put`/`Get`/`Delete` 后面**：短 URL 句柄 `/u/<sid>` 对两后端完全统一，字节都经 `Get` 流式（由取文件端点或反向取图触发）。上传 / 分析代码不分支。

> V1.4 起接口收敛为 `Put/Get/Delete/Stat/Backend`：V1.1–V1.3 的 `PublicURL`/`PutURL`/`OwnsURL`/`KeyFromURL` 全部移除（老 URL 格式不再保留）。短 URL 句柄与签名见 §4.4 V1.4。

### 4.2 Key 策略（内容寻址，天然去重）

- `ContentKey(sha256hex)` = `<sha[:2]>/<sha>`，如 `ab/abcdef...`。两级 shard 避免单目录 / 单桶前缀无限膨胀。
- 上传时已算 sha256，key 免费；相同字节 → 相同 key → `GetUploadedImageByUserAndKey` 命中则跳过 `Put`、仅 `TouchRefresh` 续期。
- 并发同字节上传的兜底：loser 在 `Insert` 唯一键冲突时回收冗余 blob，复用 winner 的行。

> **V1.1 修订（每用户独立去重，§15.1）**：把「相同字节 → 一行」改为**每用户一行**——`storage_key` 取消全局唯一、改 `(user_id, storage_key)` 复合唯一；同一用户重传同字节仍 `TouchRefresh` 自己的行，不同用户各有一行、**共享同一个 blob**（内容寻址，磁盘 / 桶仍只存一份）。删行按 `CountByKey` 引用计数，归零才删 blob。multipart 去重命中查询由全局 `GetUploadedImageByKey` 改为 `GetUploadedImageByUserAndKey`。

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

> **V1.4 修订（短 URL 句柄，本节为主）**：视觉图片 URL 改用 DB 短句柄，把上面 ~170 字符的长 URL 压到 **~52 字符**，并把 local/s3 统一为同一种句柄。新格式：
>
> ```
> {ServerAddress}/u/{shortID}?s={base64url(HMAC-SHA256(SessionSecret, method+"|"+shortID)[:12])}
> ```
>
> - `shortID`：12 字符 base62（`crypto/rand`，~71 bit），存于新列 `uploaded_images.short_id`（唯一索引，回填在 `migrateDB`）。两条插入路径（multipart `service.Upload`、`upload_image` 的 pending 行）经 `InsertWithGeneratedShortID` 生成。
> - `s`：method 绑定的截断 HMAC（12 字节 = 96bit → 16 字符 base64url，`RawURLEncoding` 无 `=`）。GET/PUT 签名空间隔离，GET token 不能当 PUT 用（替代旧 `SignURLFor("PUT",…)` 的 purpose 切分）。截断到 96bit 舒适高于 RFC2104 / NIST SP 800-107 下限（80/64bit），主防线是不可猜的 short_id，HMAC 兜底"id 泄漏"场景。
> - **URL 里不带 `expires`**：DB 行（`created_at` + retention / pending-TTL）是唯一过期权威。GET 寿命 = `UploadRetentionHours`；PUT 寿命 = `PresignedPutTTLSeconds`。故 `SignedURLTTLSeconds` 不再约束视觉 URL（仅保留作展示）。
> - 端点挂**根路由** `/u/:sid`（不在 `/api/v1` 下，以求短）：`GET` → `GetVisionFileByShortID`、`PUT` → `PutVisionFileByShortID`，均在公开组（`?s=` 即鉴权）。
> - 解析：服务端按 sid 查 `GetUploadedImageByShortID` → 取 `storage_key`+`backend` → 统一经 `Storage.Get` 流式输出（local 直读盘；**s3 也经网关流式，不再 302 到 presigned**——与反向取图路径一致，长 presigned URL 永不暴露给模型）。
> - 反向取图：`analyze_image` 对自家 `/u/` URL 经 `storage.OwnsShortURL`/`ShortIDFromURL` → 查行 → `fetchOwnImage(storage_key)`，沿用"自家 URL 反向取图不验签"约定（多一次索引查询，可忽略）。
> - 安全：capability URL（W3C 认可）+ 截断 HMAC 双层；http/https 均支持（URL 形态不绑定 scheme）——生产建议 https（`?s=` 是凭证，http 下会明文进代理/访问日志），但不强制、不告警；建议给 `/u/` 加粗粒度 per-IP 限流作纵深防御（NIST 伪造模型假设失败验证次数有上限）。
> - 老格式**已全量移除**（不再 accept-both）：老路由 `/api/v1/vision/files/*key`、老 controller、`SignURL/VerifyURL/SignURLFor/VerifyURLFor`、`GetUploadedImageByKey`、`Storage.PublicURL/PutURL/OwnsURL/KeyFromURL` 全部删除；`Storage` 接口收敛为 `Put/Get/Delete/Stat/Backend`。

### 4.5 S3 后端

- minio-go v7 覆盖 AWS S3 / MinIO / Cloudflare R2 / B2 / Alibaba OSS / Tencent COS。
- V1.4 起 S3 与 local 一致：经 `Storage.Get` 流式输出（由 `/u/<sid>` 端点或反向取图触发），不再返回 presigned GET、字节也不绕过网关。短 URL 句柄对两后端完全统一。
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
    // http 与 https 均接受。外部 URL 纯透传上游（网关永不下载，无 SSRF）；
    // 自家存储 URL 经 OwnsURL 判定后反向读本地对象字节、以 base64 发上游（V1.3）。
    if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
        return nil, fmt.Errorf("image_url must be an http(s) URL")
    }
    if UploadStore != nil && UploadStore.OwnsURL(params.ImageURL) {
        if key, ok := UploadStore.KeyFromURL(params.ImageURL); ok {
            imgBytes, mediaType, err := fetchOwnImage(ctx, key)  // Stat 校验大小 + Get 读字节 + 嗅探
            if err != nil { return nil, err }
            input.Bytes, input.MediaType = imgBytes, mediaType   // 同 camera 路径
        } else {
            input.URL = params.ImageURL                          // 自家 host 但 key 不可恢复 → 透传
        }
    } else {
        input.URL = params.ImageURL                              // 外部 URL → 纯透传上游
    }
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

### 7.4 直传端点（预签名 PUT，V1.1）

与 §7.1 multipart 路径并列的第二条上传路径。差异：**字节不经 multipart、上传 curl 不带 API Key**，专供带 Bash 的 coding agent（Claude Code / Cline 等）。完整设计见 §14。

| 路由 | 中间件 | 用途 |
|------|--------|------|
| `PUT /api/v1/vision/files/*key` | 无（purpose 签名即鉴权） | 接收 `upload_image` 预签名的裸 PUT body（**仅 local 后端**；s3 后端模型直传桶、不走此端点） |

> 与 `GET /api/v1/vision/files/*key` 同路径不同 method，gin radix-tree 可共存。

**鉴权**：HMAC 签名带 purpose 标记（`HMAC-SHA256("PUT|key|expires")`，见 §14.5），**不套 APIKeyAuth**——端点公开、签名即凭证，故模型的 curl 里没有 `sk-`。

**流程**：`VerifyURLFor("PUT",...)` → 查 `uploaded_images` 行须为 `pending`（已 `uploaded` → 409 一次性 slot）→ `http.MaxBytesReader` 有界读取 → `SniffMediaType` → `Storage.Put` → 行翻 `status=uploaded`、回填 `media_type` / `size`。

| 失败条件 | 状态码 |
|----------|--------|
| 缺 / 非法 `expires` 或缺 `token` | 403 |
| `expires` 已过期 | 410 |
| PUT purpose 签名不匹配（含用 GET token 冒充 PUT） | 403 |
| key 无 `uploaded_images` 行 / 已清理 | 404 |
| slot 已 `uploaded`（一次性） | 409 |
| 超过 `VisionUploadMaxBytes` | 413 |

**curl 端到端（直传路径）**：

```bash
# 1) 调 upload_image 拿现成 curl（JSON-RPC tools/call，节选）
curl -X POST http://localhost:3000/mcp/group/<slug> -H "X-API-Key: sk-..." -d '{
  "method":"tools/call",
  "params":{"name":"vision.upload_image","arguments":{"local_path":"./photo.jpg"}}
}'
# => 返回文本块：单条 upload_command（按 OS 从 local_path 推断 curl 或 curl.exe）+ image_url + expires_in，见 §14.2.1

# 2) 模型用 Bash 跑 upload_command（注意：无任何 API Key 头）
curl -X PUT -T './photo.jpg' 'https://.../files/a1/a1b2...?expires=...&token=...'             # 路径无盘符/反斜杠 → 推断 unix → curl
curl.exe -X PUT -T 'C:\photos\photo.jpg' 'https://.../files/a1/a1b2...?expires=...&token=...' # 路径含盘符/反斜杠 → 推断 Windows → curl.exe
# s3 后端时该 URL 指向桶，字节直传桶、不经网关

# 3) 用返回的 image_url 调 analyze_image（同 §7.3 第 4 步）
```

---

## 8. 数据模型

`uploaded_images` 表（`model/uploaded_image.go`）：

| 列 | 类型 | 说明 |
|----|------|------|
| `id` | int64 PK | |
| `user_id` | int64, index | 上传者（JWT 或 API Key 归属用户） |
| `storage_key` | varchar(128), index | 内容寻址 key `ab/<sha256>`。**V1.1**：取消全局唯一，改 **`(user_id, storage_key)` 复合唯一**（每用户独立去重）+ `storage_key` 普通索引（`CountByKey` 引用计数查询用） |
| `media_type` | varchar(64) | magic-byte 嗅探的 mime |
| `size` | int64 | 解码后字节长度 |
| `backend` | varchar(16) | `local` / `s3`——blob 所在后端 |
| `status` | varchar(16), default `uploaded` | **V1.1**：`pending`（`upload_image` 已建 slot、待 PUT）/ `uploaded`（PUT 已落地 / multipart 上传）。**默认 `uploaded`**——legacy multipart 路径不知此字段、Insert 时拿默认值即正确；只有 `upload_image` 显式置 `pending`。GORM AutoMigrate 自动加列 |
| `created_at` | time, index | **保留期时钟**：`< now - UploadRetentionHours` 即被清理 |
| `updated_at` | time | |

> ⚠️ **无 `gorm.DeletedAt`**：行是短生命周期，清理任务硬删行 + 对象。软删墓碑会破坏内容寻址去重（同字节重传会建重复行而不是复活已有 key）。

**保留期是动态的**——没有 `expires_at` 列。清理 sweep 每次 `now - UploadRetentionHours` 重算 cutoff，所以管理员**调小 `UploadRetentionHours` 在下次 tick 立即生效**，已有上传也会被回收。同字节重传走 `TouchRefresh`（重置 `created_at`）续期，热门图按最近一次请求计寿命。

> ⚠️ **V1.1 schema 迁移（须手写，AutoMigrate 不改索引）**：① 删 `storage_key` 上的旧**全局唯一索引**；② 建 `(user_id, storage_key)` **复合唯一索引**；③ 建 `storage_key` **普通索引**。`status` 列由 AutoMigrate 自动加。建议在 `migrateDB` 加一段启动迁移（检测旧索引并重建）。**存量数据无需回填**——每 key 一行恰是新模型的合法子集，迁移后多用户可各自建行。

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
| `VisionInlineMaxBytes` | `10240` | **V1.1** base64 内联软阈值：≤ 此值小图走 base64、超过则引导走上传（§16）；`0` = 关闭（退回 V1.0 行为） |
| `SignedURLTTLSeconds` | `3600` | 签名 URL 有效期，1h |
| `PresignedPutTTLSeconds` | `600` | **V1.1** 预签名 PUT URL 有效期（直传路径），须 < `SignedURLTTLSeconds`（GET 交给 analyze 的窗口要盖住上传窗口） |
| `UploadRetentionHours` | `24` | 上传保留时长，须 > `SignedURLTTLSeconds` |
| `UploadCleanupIntervalMinutes` | `60` | 过期清理扫描间隔（**改后需重启才生效**——ticker 启动时读一次） |
| `MaxUploadsPerUser` | `0` | **V1.1** 每用户活跃上传数上限（护栏，防滥用）；`0` = 不限。超限上传返回 429 |

`StorageAccessKey` / `StorageSecretKey` 已加入 `sensitiveKeys`，管理员设置接口不会回显值。

---

## 10. 后台清理任务

`service/upload_cleanup.go::StartCleanupLoop(ctx)`：

- ticker 按 `UploadCleanupIntervalMinutes` 周期触发；`ctx` 取消即退出（`router.StopBackgroundJobs` → `backgroundJobsCancel`）。
- 每次 sweep 取 `ListExpiredUploads(now - retention, 500)`（有界，防大批积压单 tick 占满），逐项 `Storage.Delete` + `DeleteUploadedImageByID`；单项失败只 log 不中止整轮（行仍删除，残留 blob 无害 / 下次幂等回收）。

接线：`cmd/server/main.go` 在 `srvCtx` 创建后调 `router.StartBackgroundJobs(srvCtx)`，`defer router.StopBackgroundJobs()`，与 `StartCloudConnections` / `StopCloudConnections` 对称。

> V1.1 在此基础上扩展：① `pending` 行（`upload_image` 建的未完成 slot）在 PUT URL 失效后**快清**（不等满 24h）；② 新增用户 / 管理员**手动删除** API（与自动清理互补）；③ 每用户上传数**护栏**。完整设计见 §15。注意：`UploadRetentionHours` 每 sweep 现读（改完下一 tick 生效），但 `UploadCleanupIntervalMinutes` 仅启动时读一次（改后需重启）。

---

## 11. 安全模型

| 威胁 | 处置 |
|------|------|
| **SSRF** | new-mcp **永不下载外部 `image_url`**，纯透传给上游；vision_handler 仅校验 `scheme ∈ {http,https} && host!=""`，无出站拉取（scheme 不构成 SSRF 面，http/https 均放行以兼容非 TLS 部署）。自家存储 URL 经 `OwnsURL` 校验为网关自己签发，反向读本地对象字节（`KeyFromURL`→`Get`）无 SSRF 面——只读自家对象、永不对外部主机发请求 |
| **文件类型欺骗** | 上传与 serve 都走 `SniffMediaType`（magic byte），不信扩展名；空 mime 拒绝 |
| **上传大小** | `http.MaxBytesReader`（controller，`FormFile` 前）+ base64 解码后 `len` 上限（vision_handler），双重强制 |
| **路径遍历**（local） | `fullpath` 清洗 key，拒 `..` / 反斜杠 / 空段 / `//`，校验 `filepath.Join(root,key)` 仍在 root 下 |
| **签名 URL 伪造** | HMAC-SHA256，`crypto/subtle` 常量时间比对；密钥 = `SESSION_SECRET`，缺则弱默认 + 启动告警 |
| **签名 URL 重放** | TTL 内接受（同 S3 presign 模型），`expires` 过期 → 410 |
| **SessionSecret 为空** | `initUploadStorage` 启动告警（避免签名 URL 用默认弱密钥）；生产应设 `SESSION_SECRET` |
| **ServerAddress 未改默认值** | local 后端 + `localhost` / `127.0.0.1` 时启动告警（上游拉不到图） |
| **清理 / 使用冲突** | 保留期(24h) > 签名 TTL(1h)，TTL 先过期上游已拉到字节，删除在后；TOCTOU 窗口可接受（上游可重试） |
| **V1.1：API Key 进模型上下文** | 直传路径的 curl 只含预签名 URL、**无 `sk-`**（预签名 URL 即凭证）；multipart 路径若由模型跑 curl 则必带 `sk-`——故 agent 自助上传场景一律走 §14 直传 |
| **V1.1：签名 URL 跨方法重放** | PUT 路径用 purpose 标记签名 `HMAC("PUT\|key\|expires")`，与 GET 的 `HMAC("key\|expires")` 空间隔离 + gin 按 method 路由；用 GET token 冒充 PUT → 403 |
| **V1.1：直传大小 / 类型强制** | S3 presigned PUT 无法在签名里绑 `content-length-range` / content-type（仅 POST policy 支持）→ 改服务端强制：local 走 `MaxBytesReader`，s3 在 analyze 前 `Stat` 校验大小、嗅探交给上游 |
| **V1.1：忘传就用 / 空跑计费** | `analyze_image` 对自家存储 URL 先 `Stat` 确认对象存在再处理，拦截「模型跳过 curl」、避免上游空跑计费；V1.3 起自家 URL 经 `KeyFromURL`→`Stat`→`Get` 反向取字节（缺失对象返回带操作指引的错误、超 `VisionUploadMaxBytes` 直接拒） |

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

**V1.1（shell 直传 + 图片管理 + 入参择优，已实现，详见 §14、§15、§16）**：

- `upload_image` per-config 内置工具（V1.2：随 `buildToolsCache` 注入每个视觉服务，固定名 `vision.upload_image`；入参 `local_path`，返回预签名 PUT curl + `image_url` + `expires_in`）
- `Storage.PutURL` / `Stat` / `OwnsURL`（V1.1）/ `KeyFromURL`（V1.3）接口扩展（local = HMAC PUT URL，s3 = `PresignedPutObject`）
- `PUT /api/v1/vision/files/*key` 直传端点 + purpose 绑定的 HMAC 签名（`SignURLFor`/`VerifyURLFor`）
- `uploaded_images.status` 状态机（`pending` / `uploaded`，默认 `uploaded`）+ `PresignedPutTTLSeconds` 配置键
- MCP `initialize` 加 `instructions` 字段 + `analyze_image` 对自家 URL 反向取字节（V1.1 为 `Stat` 确认 + 透传；V1.3 升级为 `KeyFromURL`→`Stat`→`Get`→`ImageInput{Bytes}`，见 §14.7）
- **图片管理 API**：用户列表 / 删除（`GET/DELETE /vision/uploads`、`/vision/mcp-uploads`）+ 管理员列表 / 删除（`/admin/vision/uploads`）
- **去重改每用户独立**：`(user_id, storage_key)` 复合唯一 + blob 引用计数（`CountByKey`，共享 blob、归零才删）；依据 = 图像字节 SHA-256（非文件名 / 大小）。须手写索引迁移
- **自动删除完善**：`pending` 行快清 + 手动删除与自动清理互补
- **配额护栏**：`MaxUploadsPerUser`（默认 `0` 不限）
- **`analyze_image` 入参择优**（§16）：`VisionInlineMaxBytes`（默认 10KB）软阈值 + 工具 description 分档引导，小图 base64 / 大图上传

### 13.2 待办 / 未覆盖

| 项 | 说明 |
|----|------|
| **S3 真实桶端到端** | s3 后端已 build / vet / 单测，但未对真实桶跑上传 / 拉图验证；需配 `StorageBackend=s3` + 凭证执行 §12.2 第 6 步 |
| **存量 vision 配置重新同步** | 旧 vision 配置的 `ToolsCache` 仍是旧 schema（无 `image_url`）；**需重新保存 / 启用**才 regenerate 暴露 `image_url`。新建配置直接生效 |
| **前端管理 UI** | ✅ 已实现：用户图片管理页（`/vision/uploads`，缩略图 / 复制签名 URL / 删除）+ 管理员图片管理页（`/admin/uploads`，按 user_id 筛选）+ 管理后台「存储 / 视觉上传」设置 Tab（后端选择 / S3 凭据 / 上传调优参数）。vision 测试面板的上传按钮为可选增强，未做 |
| **Gemini 原生 file_uri** | 对任意 https URL 可能不稳（V1 可靠 URL 路径为 OpenAI 兼容与 Anthropic） |
| **网关侧 resize**（可选降本） | ✅ **已实现（默认关）**：两个系统设置开关 `VisionResizeEnabled` / `VisionCompressEnabled`（+ `VisionResizeMaxEdge` 默认 1568、`VisionJPEGQuality` 默认 85）。在 `client.Analyze` 前对内存字节做 resize / 高保真重编码，磁盘原图与 GET/去重不动；URL 透传不触碰；任何解码/编码失败 **fail-open 回退原图**；「永不膨胀」守卫：重编码后若 ≥ 原图则回退原图。两开关正交：`resize` 只管尺寸，`compress` 管格式转换。compress **开**：所有格式（JPEG/WebP/PNG/GIF）→ JPEG@质量，透明区域白底合成防黑底，**GIF 仅取首帧（动画丢失）**；compress **关**（仅 resize）：PNG 仍保 PNG（保透明）、GIF 跳过（防丢动画）、其余重编码（WebP→JPEG）。另：管理员图片管理页操作列有「调优效果预览」按钮（`GET /admin/vision/uploads/:id/preview`，dry-run 跑当前设置并回原图/优化后对比）。详见 `internal/mcp/vision/transform.go` |
| ~~**StoreAddress 反向回退**~~ | ✅ **V1.3 已实现**（原 plan 遗留钩子）：自家存储 URL（`OwnsURL` 命中）经 `KeyFromURL`→`Get` 反向取字节、以 `ImageInput{Bytes}` 发上游（无 SSRF，因是自家对象），兼作本地部署兜底与 Gemini 路径的可靠替代，见 §14.7 |

### 13.3 协议趋势（短期不依赖）

MCP WG 的 `SEP-2532`（resources/stream）、`SEP-2166`（httpUrl/httpUrlExpiresAt）、issue #527 均为 Draft。截至 2026-07-28，`resources/read` 与 tool result image 仍只支持 base64——本次方案正是绕开协议未定案的过渡工程实现。

---

## 14. V1.1：shell 命令行直传（预签名 PUT）

> 状态：**已实现**。本节是叠加在 V1.0（§1–§13）之上的增量设计，不改已实现链路。

### 14.1 为什么需要第二条上传路径

V1.0 的 multipart 上传（§7.1）要求调用方带 `sk-` API Key（`/api/v1/vision/mcp-upload`）。当上传由**模型自己用 curl 执行**时（Claude Code / Cline / Roo 等带 Bash 的 agent），这条路径有硬伤——即「权限 key 问题」：

- 模型生成的 curl 必须含 `Authorization: Bearer sk-...` 或 `X-API-Key: sk-...` → **API Key 进入模型上下文、对话记录、调用日志**。
- multipart 表单（boundary / 字段名）对模型也更难正确拼装。

V1.1 新增的**预签名 PUT 直传**路径：模型调 `upload_image` 拿到一条**现成的、不含 key 的 curl 命令**，用 Bash 执行把图片直传存储，再用返回的 `image_url` 调 `analyze_image`。整条链路上模型手里只有一个用完即弃的签名 URL，**任何密钥都不暴露给模型**。

两条路径并存：multipart（§7.1）留给 Web UI / 服务端经手字节的场景；预签名 PUT（本节）留给 agent 自助上传。

### 14.2 `upload_image` per-config 内置工具（V1.2）

- **per-config 内置工具**：V1.2 起不再是全局单例，而是每个视觉服务（`vision_<id>`）的**第三个工具**（与 `analyze_image`/`describe_scene` 并列），由 `service/vision.go::buildToolsCache` 末尾追加 `virtual.UploadImageTool()`。暴露名 = `vision_<id>__vision.upload_image`（direct/group 模式，`CollectToolsForGroups` 加 `__` 前缀）；smart 模式 `tool_id` = `vision_<id>.vision.upload_image`。`ParseNamespacedName` 按 `__` 优先、其次 `.` 切分，无需特判。
- **名称与描述固定**：`vision.upload_image` 与其描述是**硬编码常量**（`virtual.UploadImageToolName` / `uploadImageDesc`），**不进 `VisionConfig` 字段、不可编辑**——区别于 `analyze_image`/`describe_scene`（二者名/描述存库、可在详情页改）。`UploadImageTool()` 每次返回**全新 map**，因为 `CollectToolsForGroups` 会就地改写 `t["name"]` 加前缀，共享实例会被污染。
- **派发**：经 per-user `VirtualToolRegistry` → `VisionHandler`（与 analyze_image 同一条路）。`VisionHandler` 加载 `vc` 后、解析 `image`/`image_url` 之前短路：`strings.HasSuffix(toolName,"upload_image")` → `handleUploadImage`。`upload_image` 入参是 `local_path`，与图片分析入参完全不同，故必须在分析参数校验之前短路。
- **调用方 userID**：`upload_image` 需调用方 userID（上传归属 + `MaxUploadsPerUser` 配额，与 multipart `UploadVisionImage`、旧全局工具语义一致，按**调用方**计）。`VirtualToolHandler` 签名无 userID（改它会波及 camera），故用 context 传递：`gateway_handler` 在 `routeAndCall` / `handleExecute` 两处 virtual 派发点调 `virtual.WithCallerUserID(ctx, logCtx.UserID)`；`VisionHandler` upload 分支 `CallerUserID(ctx)` 读取，`0` 时回退 `vc.UserID`。

**入参**：

```json
{ "local_path": "/abs/path/to/photo.jpg" }
```

> `local_path` 仅用于拼 curl 占位，**服务端读不到客户端盘**；文件得由模型在本地用 Bash 上传。

**返回值**（核心是 `upload_command`（按 OS 推断、单条）+ `image_url`）：

实际返回是 MCP `tools/call` 的 `content[].text` 文本块（节选）：

```text
Upload slot ready. Run upload_command, then call vision.analyze_image with the image_url below.

upload_command: curl.exe -X PUT -T 'C:\abs\path\photo.jpg' 'https://host/u/8QkP2mR9xY4a?s=Kp9bZx7mQ2nLr5tA'
image_url: https://host/u/8QkP2mR9xY4a?s=Vc3Nq8wYj4Hk6sTp
expires_in: 600s
```

PUT 与 GET 共用同一个 `shortID`（`8QkP2mR9xY4a`），仅 `?s=` 签名不同（method 绑定：PUT-MAC ≠ GET-MAC，不可互换）。unix 路径（如 `/Users/me/x.png`）下 `upload_command` 里是 `curl`（无 `.exe`）。**URL 与后端无关**：local 与 s3 都返回同一种 `/u/<sid>?s=…` 句柄——s3 的 GET 由网关内部现签 presigned 再 302（长 presigned URL 不暴露给模型），PUT 经网关转发到桶（仍无 key）。path+query 恒 32 字符，加 host 约 52 字符（旧格式约 170）。详见 §4.4 V1.4。

### 14.2.1 跨系统命令生成（Windows PowerShell / macOS / Linux）

模型在未知 Shell + 未知 OS 下执行返回的命令；唯一会失败的环境是 **Windows PowerShell**——它把 `curl` 别名成 `Invoke-WebRequest`，拒绝 `-T`/`-X`。对策（`internal/mcp/virtual/upload_image.go`）：

1. **从 `local_path` 推断 OS**（`looksLikeWindowsPath`）：含反斜杠或盘符前缀（`C:\` / `C:/`）→ Windows；否则 → unix。Windows 绝对路径一目了然，unix 路径实践上不含反斜杠 / 盘符，故可靠。
2. **二进制名分流**：Windows → `curl.exe`（绕开别名，cmd / Git Bash 同样可用；Win10 1803+ 自带）；macOS / Linux / Git Bash → `curl`（真 curl）。
3. **单引号包参**（`quoteArg`）：空格与 `&`/`=`/`?` 等 shell 元字符在 PowerShell / bash / zsh 里都是字面量。Windows 文件名禁止单引号、HMAC/presign URL 也不含引号，故无需转义；不用 Go `%q`（它会把 `\` 双转义成 `\\`，在 PowerShell/cmd 里路径反而错）。
4. **只返回检测到的那一条**（`uploadCommand`）：路径判定为 Windows → 给 `curl.exe`，否则 → 给 `curl`，**单条**，不再附带另一系统的命令或换名提示。两型仅二进制名不同、长 presigned URL 完全一致，多给一条是纯噪音；检测对绝对路径几乎总对，极少数推断错（相对路径、Git Bash 风格 `/c/...`）时 agent 自行 curl↔curl.exe 一词替换即可——与本工具出现前同样的单次重试。

「路径推断」零侵入——现有所有 agent 不改调用即生效；故**不新增 `platform` 参数**（如需强制覆盖，后续可加可选参数，回落到路径推断）。

### 14.3 Storage 抽象扩展（PutURL / Stat / OwnsURL）

`internal/storage/storage.go` 接口加三个方法（既有 `Put/Get/Delete/PublicURL/Backend` 不变）：

```go
type Storage interface {
    // ... 既有方法 ...
    PutURL(ctx context.Context, key string, ttl time.Duration) (string, error) // 预签名 PUT URL
    Stat(ctx context.Context, key string) (ObjectInfo, error)                  // 不读 body 的元信息
    OwnsURL(rawurl string) bool                                                // 是否自家存储
}

type ObjectInfo struct {
    Size      int64
    MediaType string // best-effort；local 留空（行是权威）
}
```

| 方法 | local 后端 | s3 后端 |
|------|-----------|---------|
| `PutURL` | HMAC 签名 PUT URL，指向 `PUT /api/v1/vision/files/*key` | `client.PresignedPutObject(ctx, bucket, obj, ttl)` |
| `Stat` | `os.Stat`（size） | `client.StatObject`（size + ContentType） |
| `OwnsURL` | host == `ServerAddress` 且路径以 `/api/v1/vision/files/` 开头 | host == 配置的 s3 端点 host |

> **S3 presigned PUT 固有限制**：`PresignedPutObject` 只签 method+bucket+object+expires，**无法绑 `content-length-range` / content-type**（那是 `PresignedPostPolicy` 的 POST 表单能力，curl 形态不同、破坏统一性，不用）。大小/类型改服务端强制：local 走 `MaxBytesReader`，s3 在 §14.7 analyze 前 `Stat` 校验。可选地用 `PresignHeader(... "Content-Type")` 绑死类型，但服务端猜错会拒掉合法上传，**不推荐**。

### 14.4 Key 策略分歧：UUID，放弃去重

| 路径 | key | 去重 |
|------|-----|------|
| multipart（V1.0） | 内容寻址 `<sha[:2]>/<sha>` | 天然去重 |
| 预签名 PUT（V1.1） | **UUID** `<uuid[:2]>/<uuid>` | **不去重** |

直传路径里服务端在上传前**看不到字节**，无法算 sha256 → 只能用 UUID。每张图一个新 slot / 新 blob；图片 24h 短生命周期，可接受。两条路径 key 空间隔离、互不影响；`storage_key` 唯一索引对 UUID 同样成立。

### 14.5 purpose 绑定的 HMAC 签名（防跨方法重放）

V1.0 的 `SignURL(key, expires)` 只签 `key|expires`，**不绑 HTTP method**。V1.1 新增的 PUT 端点与 GET 同路径 `*key`，若复用同一签名，一条 GET 签名 URL（会发给第三方上游、有泄漏面）就能被改用 PUT 覆盖对象。

**修复**：PUT 路径用 purpose 标记签名，GET 路径维持不变（向后兼容在流通的 GET URL）：

```go
// sign.go 新增（SignURL / VerifyURL 保留不动）
func SignURLFor(purpose, key string, expires int64) string   // HMAC-SHA256(purpose + "|" + key + "|" + expires)
func VerifyURLFor(purpose, key string, expires int64, token string) bool
```

两个 HMAC payload（`"PUT|key|expires"` vs `"key|expires"`）不相交 + gin 按 method 路由 → GET token 冒充 PUT 会落到 PUT handler、`VerifyURLFor("PUT",...)` 失败 → 403，反之亦然。

### 14.6 本地后端 PUT 端点

`controller/upload.go` 加 `PutVisionFile`，挂公开组（`router/api_router.go`）：

```
PUT /api/v1/vision/files/*key   # 无 APIKeyAuth，purpose 签名即鉴权
```

流程（镜像 `GetVisionFile` 结构）：

```
VerifyURLFor("PUT", key, expires, token)  →  不通过 403 / 过期 410
→ model.GetUploadedImageByKey(key)        →  无行 404 / 已 uploaded 409（一次性 slot）
→ http.MaxBytesReader(VisionUploadMaxBytes)
→ io.ReadAll → vision.SniffMediaType      →  空 mime 415
→ service.UploadStore.Put(key, reader, mediaType)
→ model.MarkUploaded(key, mediaType, size)→  行翻 status=uploaded
→ 200 {status:"uploaded", key, size, mime}
```

> s3 后端模型直传桶、**不经此端点**；行在 `upload_image` 时已建为 `pending`，由 §14.7 的 `Stat` 在 analyze 时确认 + 回填。

### 14.7 analyze_image 对自家 URL 反向取字节（V1.3）

`internal/mcp/virtual/vision_handler.go` 的 `image_url` 分支，在 http(s) 校验后判 `OwnsURL`：自家存储 URL 反向读字节、以 base64 发上游；任意外部 http(s) URL 仍是纯透传、无 SSRF。

```go
if UploadStore != nil && UploadStore.OwnsURL(params.ImageURL) {
    if key, ok := UploadStore.KeyFromURL(params.ImageURL); ok {   // local: 从路径剥 key；s3: 从 presign 路径取后缀
        imgBytes, mediaType, err := fetchOwnImage(ctx, key)       // Stat(大小) → Get → io.ReadAll → SniffMediaType
        if err != nil { return nil, err }                         // 缺失对象 → 「先跑 upload_command」；超限 → 直接拒
        input.Bytes, input.MediaType = imgBytes, mediaType         // 同 camera 路径，上游直收 base64
    } else {
        input.URL = params.ImageURL                                // 自家 host 但 key 不可恢复 → 透传
    }
} else {
    input.URL = params.ImageURL                                    // 外部 URL → 纯透传上游
}
```

`fetchOwnImage`（同文件）：`Stat` 先做 `VisionUploadMaxBytes` 大小校验（Get 前拒大，不流式读超大对象）→ `Get` → `io.ReadAll` → `vision.SniffMediaType`（空 mime 拒）；`storage.ErrObjectNotFound` 转成带操作指引的错误。注意 `VisionInlineMaxBytes` **不适用**——该软阈值约束的是 base64 进「调用方 LLM 上下文」；这里字节走 new-mcp→上游，调用方只持有短 `image_url`，字节不进其上下文。

**为什么反向取字节而非透传 URL**：本地部署时上游 provider（OpenAI / Claude / Gemini）拉不到 `ServerAddress`（localhost / 内网不可达），原「上游自己 GET」路径必败；Gemini 原生 `file_uri` 对任意 URL 也不稳。自家 URL 经 `OwnsURL` 校验是网关自己签发的，读本地对象无 SSRF 面。安全不变量仍成立：**new-mcp 永不下载外部 URL**。

`service/vision.go::buildToolsCache` 的 `image_url` description 同步更新：把「获取方式」从 multipart 端点改为优先指向 `upload_image`。base64 兼容与外部 URL 透传都不变（功能不变）。

### 14.8 「告诉模型」的四层引导

模型不会自己猜流程，靠四个渠道叠加（coding agent 对工具返回的操作指引服从度极高）：

1. **工具返回值**（最有效）：`upload_image` 返回里直接给 `upload_command`（现成 curl）+ `next_step`，模型读到就照做。
2. **工具 description**：声明完整工作流（`upload_image` → Bash curl → `analyze_image(image_url)`），并明令「不要把 base64 塞参数」。
3. **MCP `initialize` 的 `instructions`**：当前 `handleInitialize` 未返回该字段，V1.1 在 Result map 里加全局工作流说明（smart 模式 tools/list 只有 3 个 meta 工具，这里是其获知 `vision.upload_image` 的主渠道）。
4. **错误兜底**：模型跳步（拿本地路径 / 无效 URL 调 analyze）时，返回带「1) `upload_image` → 2) Bash curl → 3) `analyze_image(image_url)`」的操作指引错误，促其自纠。

### 14.9 两后端字节走向 + 时序

**字节走向**（须明确，避免误解）：

| 后端 | 模型 curl 目标 | 字节是否经网关 |
|------|---------------|----------------|
| s3 | 桶 presigned PUT URL | **否**，直传桶 |
| local | `PUT /api/v1/vision/files/*key` | **是**（网关即存储），但是无 key 的裸 PUT |

两种情况模型看到的 curl 形态完全一致，行为对模型无差异。

**时序**：

```
模型: tools/call vision.upload_image(local_path)
  └─ 网关: 校验调用方 API Key → 生成 uuid key → 建 uploaded_images 行(status=pending)
          → Storage.PutURL(key, PresignedPutTTL) + Storage.PublicURL(key, SignedURLTTL)
模型 ← { upload_command, image_url, expires_in }
模型: Bash 执行 upload_command            # 无 key；s3 直传桶 / local 命中 PUT 端点
模型: tools/call vision.analyze_image(image_url=<返回的 image_url>)
  └─ 网关: OwnsURL 命中 → KeyFromURL 剥 key → Stat 大小校验 → Get 读字节 → 嗅探 mime
          → ImageInput{Bytes} 发上游 → 上游直收 base64（无需回访 ServerAddress）
模型 ← 分析结果
```

> 两层鉴权都不让模型接触密钥：① Agent↔MCP 走客户端配置里的 API Key（模型不可见）；② 上传↔存储走预签名 URL（无 key）。存储密钥只在网关、MCP token 只在客户端配置，模型手里只有一个用完即弃的签名 URL。

### 14.10 新增 / 修改文件（V1.1 增量）

**新增**：

| 文件 | 职责 |
|------|------|
| `internal/mcp/virtual/global.go` | `GlobalTools`（`upload_image` schema）+ `HandleGlobalTool` 派发 + `UploadImageToolName` |
| `internal/mcp/virtual/upload_image.go` | `handleUploadImage`：uuid key → 建 pending 行 → `PutURL` + `PublicURL` → 拼 curl + 结构化返回 |

**修改**：

| 文件 | 改动 |
|------|------|
| `internal/storage/storage.go` | 接口加 `PutURL` / `Stat` / `OwnsURL`（V1.1）/ `KeyFromURL`（V1.3）+ `ObjectInfo` |
| `internal/storage/s3.go` | 四方法实现（`PresignedPutObject` / `StatObject` / 端点 host 比对 / 从 presign 路径剥 key） |
| `internal/storage/local.go` | 四方法实现（HMAC PUT URL / `os.Stat` / `ServerAddress`+路径比对 / 从文件 URL 路径剥 key） |
| `internal/storage/sign.go` | 加 `SignURLFor` / `VerifyURLFor`（purpose 绑定）；`SignURL` / `VerifyURL` 不动 |
| `controller/upload.go` | 加 `PutVisionFile`（复用 `UploadService` 有界读 + 嗅探） |
| `router/api_router.go` | 公开组加 `PUT /vision/files/*key` |
| `model/uploaded_image.go` | 加 `Status` 字段 + 常量 + `MarkUploaded` |
| `model/option.go` | `defaultOptions` 加 `PresignedPutTTLSeconds`（600） |
| `service/vision.go` | `imageURLDesc` 文案指向 `upload_image` |
| `internal/mcp/virtual/vision_handler.go` | 自家 URL 分支：V1.1 `Stat` 确认 → V1.3 反向取字节（`KeyFromURL`→`Stat`→`Get`→`ImageInput{Bytes}`），删 `ownStorageKeyFromURL`、加 `fetchOwnImage` |
| `internal/mcp/handler/gateway_handler.go` | `initialize` 加 `instructions`；tools/list 追加全局工具；`routeAndCall` + `handleExecute` 短路派发 |

> 零新增依赖：`google/uuid v1.6.0`、`minio-go/v7 v7.2.1` 均已在 `go.mod`。

**V1.2 增量**（`upload_image` 由全局工具改为 per-config 内置工具，详见 §14.2）：

| 文件 | 改动 |
|------|------|
| `internal/mcp/virtual/global.go` | **删除整文件**（`GlobalTools`/`IsGlobalTool`/`HandleGlobalTool` 全删） |
| `internal/mcp/virtual/upload_image.go` | 迁入 `UploadStore`/`UploadImageToolName`；新增 `UploadImageTool()`（每次返回新 map）+ `WithCallerUserID`/`CallerUserID` context 辅助；`handleUploadImage` 逻辑不变 |
| `service/vision.go::buildToolsCache` | 末尾追加 `virtual.UploadImageTool()`（第三个工具，固定名/描述） |
| `internal/mcp/virtual/vision_handler.go` | 加载 `vc` 后加 `upload_image` 早返回分支（→ `handleUploadImage`，userID 从 ctx 取、回退 `vc.UserID`） |
| `internal/mcp/handler/gateway_handler.go` | 删 `handleToolsList` 两处 `GlobalTools` 追加；删 `routeAndCall`/`handleExecute` 全局短路块；两处 virtual 派发点注入 `virtual.WithCallerUserID(ctx, logCtx.UserID)` |
| 前端 `vision-detail-page.tsx` + `i18n/{zh,en}.json` | 详情页新增只读 `upload_image` 卡片（标题/简介/「系统内置·不可修改」徽标） |

### 14.11 验证

实现后端到端（接入 §12.2 清单）：

1. `upload_image(local_path)` → 拿 `upload_command` + `image_url` → Bash PUT → `analyze_image(image_url)` 端到端，**local 与 s3 两后端各跑一次**。
2. 篡改 PUT 签名 token → 403；`expires` 过期 → 410。
3. 用一条 **GET** 签名 URL 冒充 **PUT** → 403（验证 §14.5 purpose 绑定）。
4. 一次性 slot：对同一 key 二次 PUT → 409。
5. 模型跳步：直接拿本地路径 / 未上传的 `image_url` 调 `analyze_image` → 返回带操作指引的错误（且不发计费请求到上游）。
6. 清理：`pending` 行到期（PUT TTL 过后）被回收；已 `uploaded` 行按 `UploadRetentionHours` 回收。
7. smart 模式：`initialize` 的 `instructions` 出现；`mcp.execute vision_<id>.vision.upload_image` 与直调（`vision_<id>__vision.upload_image`）均可通。

---

## 15. 上传图片管理与自动删除

> 状态：**已实现**。V1.0 只有「上传 + 时间驱动清理」（§7、§10），无任何列表 / 删除 / 配额能力。本节补齐管理面。

### 15.1 归属与去重模型（每用户独立 + blob 共享）

- **归属**：每行 `uploaded_images.user_id` = 上传者（JWT 或 API Key 解析，见 `UploadVisionImage` 的 `user_id` → `api_key_user_id` 回落）。管理（列表 / 删除）一律按 `user_id` 过滤——用户只看得到、删得到自己的行。
- **每用户独立去重（V1.1）**：`(user_id, storage_key)` 复合唯一。同一用户重传同字节 → 命中**自己**的行 `TouchRefresh`、不建新行；**不同用户传同字节 → 各有一行**。于是每张你上传的图都在你名下、可列表 / 可删除——消除 V1.0「借用他人行」的边角。
- **blob 共享 + 引用计数**：key 仍是内容寻址（`<sha[:2]>/<sha>`），不同用户的同字节图**共享磁盘 / 桶里同一份 blob**。上传时 `CountByKey(key) > 0` → blob 已存在、**跳过 `Storage.Put`**（不重复写），只建本用户行；删行时 `CountByKey(key)` 归零才 `Storage.Delete(key)`，>0 保留。→ 不重复存、不留孤儿 blob、不误删共享 blob。
- **去重依据 = 图像字节的 SHA-256**（解码后的原始字节），**不是文件名、不是字节大小**——文件名客户端可乱填、不可信；字节大小会撞（两张不同的图可能同尺寸）。SHA-256 对任意字节唯一，是可靠的内容指纹。

### 15.2 用户侧管理 API

双鉴权（JWT + API Key），镜像 upload 端点的分路由模式（gin radix-tree 不允许同路径挂两套中间件）：

| 路由 | 中间件 | 用途 |
|------|--------|------|
| `GET /api/v1/vision/uploads` | `UserAuth`（JWT） | Web UI 列出自己的上传 |
| `GET /api/v1/vision/mcp-uploads` | `APIKeyAuth` + `RateLimit` | MCP 客户端列出 |
| `DELETE /api/v1/vision/uploads/:id` | `UserAuth` | 删除自己的一个 |
| `DELETE /api/v1/vision/mcp-uploads/:id` | `APIKeyAuth` + `RateLimit` | MCP 客户端删除 |

身份解析同 `UploadVisionImage`：`c.GetInt64("user_id")` → 回落 `c.GetInt64("api_key_user_id")`。

**`GET` 响应**（分页，复用 `common.GetPagination` / `common.PageOf`）：

```json
{
  "success": true,
  "data": {
    "items": [
      {"id":12,"key":"ab/abcd...","url":"<现签 GET URL>","mime":"image/png",
       "size":12345,"backend":"local","status":"uploaded",
       "created_at":"2026-08-12T10:00:00Z","expires_at":"2026-08-13T10:00:00Z"}
    ],
    "page":1,"page_size":20,"total":42
  }
}
```

- 查询：`WHERE user_id = ? AND status = 'uploaded' ORDER BY created_at DESC`（`pending` slot 不展示给用户）。
- `url` **每次列表现签**（`Storage.PublicURL(key, SignedURLTTLSeconds)`）——签名 URL 短命（1h），存盘的旧 URL 早过期。
- `expires_at` = **估算** `created_at + UploadRetentionHours`，仅供参考（实际以清理 tick 为准；同字节重传会 `TouchRefresh` 续期、寿命重置）。

**`DELETE /:id`**：查行 → **校验 `row.UserID == actingUserID`**（非本人返回 **404**，不泄漏存在性）→ `DeleteUploadedImageByID(id)` → `CountByKey(row.StorageKey)`，**归零才** `Storage.Delete(key)`（>0 说明他人也引用此 blob，保留）→ 200。

### 15.3 管理员侧 API

镜像 `/admin/...` 既有模式（`AdminAuth`，要求 `IsAdminRole(role)`，即 `super_admin` / `admin`）：

| 路由 | 中间件 | 用途 |
|------|--------|------|
| `GET /api/v1/admin/vision/uploads` | `AdminAuth` | 分页列出**所有用户**的上传，可带 `?user_id=` 过滤 |
| `DELETE /api/v1/admin/vision/uploads/:id` | `AdminAuth` | 删除任意用户的上传（滥用处置 / 维护） |

- 列表字段同 §15.2，额外带 `user_id` / `username`。
- 管理员删除**无归属校验**。

### 15.4 自动删除（完整）

**时间驱动**（既有 `service/upload_cleanup.go`，V1.1 扩展）：

| 项 | 行为 |
|----|------|
| 间隔 | `UploadCleanupIntervalMinutes`，**启动时读一次**（改后需重启） |
| 保留期 | `UploadRetentionHours`，**每 sweep 现读**（管理员改完下一 tick 生效） |
| 批次 | `ListExpiredUploads(cutoff, 500)`，最旧优先，有界防积压 |
| 删除 | 逐项 `DeleteUploadedImageByID(id)` → `CountByKey(key)` 归零才 `Storage.Delete(key)`（共享 blob 引用计数，§15.1）；单项失败只 log 不中止整轮 |
| 行 | 硬删（无 `DeletedAt` 墓碑，保去重干净） |

**两后端删除语义**：local = `os.Remove(filepath)`；s3 = `client.RemoveObject`（幂等，`NoSuchKey` 已映射 `ErrObjectNotFound` 静默）。

**保留期 > 签名 TTL**：`UploadRetentionHours(24h)` > `SignedURLTTLSeconds(1h)`，保证 TTL 先过期时上游已拉到字节、删除在后；TOCTOU 窗口上游可重试。

**V1.1 `pending` 行快清**（新增）：`upload_image` 建的 `pending` slot 若模型一直没跑 curl，应在 PUT URL 失效后尽快回收（不等满 24h）：

```
cutoffPending := now - PresignedPutTTLSeconds - 5min(grace)
ListPendingExpired(cutoffPending, N) → 逐项删
  # pending 从未 PUT 成功，local 无文件 / s3 无对象，Storage.Delete 静默即可
```

**手动删除**（§15.2 / §15.3）与自动清理**互补**：手动立即释放、自动兜底漏网；两者共用同一 `Storage.Delete` + 行删除路径。

### 15.5 配额护栏（可选，防滥用）

- **现状**：上传**不计费**——`billing/` 只管 MCP 工具调用配额（`PreConsume`/`Confirm`/`Refund`），完全不碰 `uploaded_images`；`RateLimit` 中间件只限 `tools/call` 频率、按用户组分桶，不限存储；全代码无任何上传计数 / 字节求和。
- **V1.1 护栏**（轻量、默认关）：`MaxUploadsPerUser`（默认 `0` = 不限）。`upload_image` 与 multipart 上传前 `CountUploadsByUser(userID)`，超限返回 **429**（提示先删旧图或等自动清理）。
- **存储字节配额**（`SumBytesByUser`）更精准但更重，留作未来商业化（接 `billing/`）的钩子，V1.1 不做。

### 15.6 新增 model 方法 / 配置键 / 文件

**`model/uploaded_image.go` 新增查询**：

| 方法 | 用途 |
|------|------|
| `GetUploadedImageByUserAndKey(userID, key) (*UploadedImage, error)` | **multipart 每用户去重命中**（替代 `GetUploadedImageByKey` 的去重用途） |
| `CountByKey(key) (int64, error)` | **blob 引用计数**：删行后 >0 留 blob、=0 才删 blob（§15.1） |
| `ListUploadsByUser(userID, offset, limit) (rows []UploadedImage, total int64, err)` | 用户列表（仅 `status='uploaded'`，`created_at DESC`） |
| `ListAllUploads(filterUserID, offset, limit) (...)` | 管理员列表（`filterUserID=0` 表示全部） |
| `CountUploadsByUser(userID) (int64, err)` | 配额护栏 |
| `ListPendingExpired(olderThan, limit) ([]UploadedImage, err)` | `pending` 快清（`WHERE status='pending' AND created_at < ?`） |

> 既有 `GetUploadedImageByKey(key)` 语义改为**返回任一行**：公开 GET 端点（`GetVisionFile`）用它做 key 存在性检查——同 key 的 blob 共享，任一行即可。**去重**用途改用 `GetUploadedImageByUserAndKey`。V1.1 直传路径用 UUID key（全局唯一），其 `PutVisionFile` 查 pending 行不受影响。

**`model/option.go` 新增**：`MaxUploadsPerUser`（默认 `0`）。

**新增 / 修改文件**：

| 文件 | 改动 |
|------|------|
| `controller/upload.go` | 加 `ListUploads` / `DeleteUpload` / `AdminListUploads` / `AdminDeleteUpload`（复用 `common.GetPagination`/`PageOf`、`Storage.PublicURL` 现签、`Storage.Delete`） |
| `model/uploaded_image.go` | 加 §15.6 四个查询方法 |
| `router/api_router.go` | auth 组加 `GET/DELETE /vision/uploads`；公开组（`APIKeyAuth`+`RateLimit`）加 `GET/DELETE /vision/mcp-uploads`；admin 组加 `GET/DELETE /admin/vision/uploads` |
| `service/upload_cleanup.go` | 加 `pending` 快清 sweep（与既有 expired sweep 同 ticker、分两段） |
| `service/upload.go` + `internal/mcp/virtual/upload_image.go` | 上传前 `CountUploadsByUser` 配额检查 |
| `model/option.go` | 加 `MaxUploadsPerUser` |

### 15.7 验证

1. 用户列自己的上传（不含他人、不含 `pending`），分页 + `url` 现签可用。
2. 用户删自己的 → 200 + blob 消失（local 文件删 / s3 对象删）；删他人的 → **404**（不泄漏）。
3. 管理员列表全部 / 按 `?user_id=` 过滤；管理员删任意用户的。
4. 自动清理：调小 `UploadRetentionHours` → 下一 tick 旧行 + blob 删；`pending` 行在 `PresignedPutTTLSeconds + grace` 后被快清。
5. 配额：设 `MaxUploadsPerUser=5`，第 6 次上传（multipart 与 `upload_image` 均算）→ 429；删除一个后恢复。

---

## 16. analyze_image 入参决策：小图 base64 / 大图上传

> 状态：**已实现**。优化 `analyze_image` 的提示词与参数描述，让模型按图片尺寸择优：小图直接 base64 内联（省一次上传往返），大图走 §14 的 `upload_image` → URL（省上下文）。

### 16.1 为什么改

V1.0 文案把 `image_url` 标为「preferred」、base64 标为「大图不推荐」——但这对**小图是过度设计**：一张小图标 / 缩略图 / 二维码走 base64 内联更简单（无上传往返、不占存储 slot、无需清理）。V1.1 改为**按尺寸分档**，让模型自己挑，两条路各得其所。

### 16.2 决策规则

```
分析前先用 Bash 查本地文件大小（stat -c%s <path> / ls -l <path> / wc -c <path>）：
- ≤ VisionInlineMaxBytes（默认 10KB）→ 小图：base64 内联，调 analyze_image(image=<base64>)。
- VisionInlineMaxBytes < size ≤ VisionUploadMaxBytes(10MB) → 大图：upload_image → Bash curl 直传 → analyze_image(image_url)。
- > VisionUploadMaxBytes → 超限，两路都拒，提示压缩或换图。
```

**阈值依据**：base64 上下文占用 ≈ 原始字节 × 1.33，且模型得把它当 **output token** 生成（比 input 贵 ~5×、更慢）——每 KB 原图 ≈ ~400 token。`VisionInlineMaxBytes` 默认 **10KB**（≈ ~4k token，覆盖图标 / 二维码 / 小缩略图；截图 / 照片走上传）。可调：图标多的场景调大、省上下文优先的场景调小；`0` = 关闭软引导、base64 许可到硬上限（退回 V1.0 行为）。`VisionUploadMaxBytes`（10MB）是两路共同**硬上限**，不动。

### 16.3 三层落实

复用 §14.8 的引导通道 + 服务端兜底：

1. **工具 description**（`service/vision.go::buildToolsCache`）：
   - `image`（base64）：「**仅小图用**（≤ `VisionInlineMaxBytes` ≈ 10KB）。大图改用 `upload_image` → `image_url`，否则撑爆上下文。」
   - `image_url`：「**大图用**。用 `upload_image` 获取（返回 curl + image_url）；小图直接 base64 内联即可。」
   - `upload_image`：「把本地**大图（> 内联阈值）**上传到存储换 URL；小图直接 base64 内联调 `analyze_image`。」
2. **`initialize` instructions**（§14.8 渠道 3）：写明「先查文件大小，小图 base64、大图 upload」的分档流程。
3. **服务端软上限（错误即指令）**：`vision_handler` 收到 base64 时，若 `VisionInlineMaxBytes > 0 && len(imgBytes) > VisionInlineMaxBytes`，返回引导而非静默接受：
   > `"图片 X 字节超过内联阈值 Y；请改走 upload_image → curl → image_url（功能不变、更省上下文）"`

   模型收到会自动改走上传。`VisionUploadMaxBytes`(10MB) 仍是**硬拒**（超限直接报错），检查顺序：先软阈值引导、再硬上限拒绝。

### 16.4 与既有约束的关系

- **功能不变**：base64 与 URL 两路都保留，只是把「无脑 URL」改成「按尺寸择优」。
- `VisionInlineMaxBytes` 只影响 base64 入参路径的软引导 / 报错；URL 路径不受影响。V1.3 自家 URL 反向取字节走 `ImageInput{Bytes}`，但字节流向是 new-mcp→上游、不进调用方 LLM 上下文，故 `VisionInlineMaxBytes` 同样不适用（只有 `VisionUploadMaxBytes` 硬上限生效）。
- base64 硬上限仍是 `VisionUploadMaxBytes`；超限两路一致拒绝。
- §6 的 `vision_handler` 分派（`image` / `image_url` 二选一）不变，§16.3 只在 base64 分支加一个软阈值检查。

### 16.5 新增 / 修改

| 文件 | 改动 |
|------|------|
| `model/option.go` | 加 `VisionInlineMaxBytes`（默认 `10240`，`0` = 关闭） |
| `service/vision.go::buildToolsCache` | 更新 `image` / `image_url` / `upload_image` 三段 description（措辞见 §16.3） |
| `internal/mcp/virtual/vision_handler.go` | base64 分支加 `VisionInlineMaxBytes` 软上限引导报错（在现有 `VisionUploadMaxBytes` 硬上限检查之前） |
| `internal/mcp/handler/gateway_handler.go` | `initialize` instructions 追加分档流程（并入 §14.8 渠道 3） |

### 16.6 验证

1. 小图（< 10KB）：base64 内联调通，一次成功。
2. 大图（10KB–10MB）：base64 调用 → 收到软引导报错 → 模型改走 `upload_image` → 成功。
3. 超大图（> 10MB）：两路均硬拒。
4. `VisionInlineMaxBytes=0`：base64 许可到 10MB（退回 V1.0 行为）。
5. 调阈值后，对应 size 的图在两条路径间正确切换。