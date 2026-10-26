# 内置图片生成 MCP

在「图片生成」页面创建配置，每条配置绑定一个模型。先获取模型或手动填写模型名，再显式测试出图，保存并启用，最后在分组中添加 `imagegen_<配置 ID>` 服务。获取模型只读取模型列表，不生成图片；测试出图会受理后台任务并调用上游。网页测试与 MCP 共用持久化任务，短等待结束后通过任务 ID 继续查询。

需要文生视频或单图生成视频时，使用独立的[视频生成 MCP](./VIDEO-GENERATION.md)。视频复用配置、上传和分组交互，采用持久化异步任务与状态查询；验收范围见[视频验收记录](./VIDEO-GENERATION-ACCEPTANCE.md)。

## 配置

- `provider`：`openai`、`gemini` 或 `openrouter`，配置列表与添加、编辑页面统一显示为 OpenAI、Gemini、OpenRouter。OpenAI 兼容模式可连接 New API 图片接口；Gemini 使用原生 `generateContent`；OpenRouter 使用独立图片 API。协议由配置显式选择，不根据模型名称推断。
- `endpoint_url`：可留空使用当前协议的官方地址。OpenAI 为 `https://api.openai.com/v1`，Gemini 为 `https://generativelanguage.googleapis.com/v1beta`，OpenRouter 为 `https://openrouter.ai/api/v1`。支持网关根地址、反向代理路径前缀及已有版本后缀；OpenRouter 官方根地址自动补 `/api/v1`，其他网关根地址补 `/v1`。更新时省略字段保留已有地址，传空字符串恢复当前协议官方地址。页面预览最终地址。
- `operation`：`generate`、`edit`、`both`（默认）。能力由配置和上游决定，不根据模型名猜测。
- `edit_request_format`：OpenAI 兼容编辑接口格式。新建 OpenAI 兼容配置默认 `generations_json`，请求 `/v1/images/generations` 并传入顶层 `image` 数组；`multipart` 请求 `/v1/images/edits`；`generations_json_extra_body` 请求生成接口并传入 `extra_body.image`。按实际网关文档选择，已有配置保持所选格式，旧配置中未设置该字段时保持 multipart；Gemini 使用原生接口。
- `timeout_seconds`：默认 180 秒，控制后台生成阶段的上游请求；0 表示不设置生成超时，仍受服务关闭影响。客户端期限、智能/批量 `timeout_ms` 和断开连接只结束受理前的校验或本次短等待，已入库任务继续执行。保存阶段独立限制为 5 分钟。
- `default_params`：JSON 对象。调用参数覆盖默认值，`extra_body` 深度合并；未填写的可选参数不发送。
- API Key 只保存在独立配置中。列表、详情和虚拟服务配置不回显密钥；编辑时留空保留原密钥。

「获取模型」仅提交协议、地址及凭证，不要求先填写配置名称、模型名或合法的生成默认参数。OpenRouter 读取专用图片模型目录，其他协议读取原有模型接口。搜索框默认 `image`，可修改或清空，也可手动填写模型 ID；不按模型名过滤或动态推断能力。

### MCP 客户端提前超时

此前同步图片生成会把 MCP 客户端取消传递给上游等待，因此 Cherry Studio 约 60 秒断开时，即使配置为 180 秒，也可能出现 `context canceled`。Cherry Studio 2.1.6 的 [MCP 超时策略源码](https://github.com/CherryHQ/cherry-studio/blob/v2.1.6/src/main/ai/mcp/mcpRequestOptions.ts)默认使用 60 秒，`timeout=0` 也回退到 60 秒。

现在生成工具默认只等待 10 秒（可选 0–30 秒），未完成时正常返回 `task_id`。Agent 应调用同配置的 `get_image_status`，不要再次调用生成工具轮询。客户端取消不影响已入库任务；配置的 180 秒在后台继续生效。如果连任务 ID 都未收到，可查看任务历史，或用相同 `request_id` 和输入重放提交以找回任务。参考图加载和校验发生在受理前，最多 30 秒；此阶段取消不会提交生成。

OpenAI 默认参数示例（实际支持的参数、尺寸、质量由上游模型决定）：

```json
{
  "size": "2K",
  "extra_body": {"seed": 42, "watermark": false, "prompt_extend": true}
}
```

Gemini 示例：

```json
{
  "aspect_ratio": "16:9",
  "image_size": "2K",
  "extra_body": {"generationConfig": {"temperature": 0.4}}
}
```

OpenAI 扩展字段平铺到 JSON 或 multipart 表单；Gemini 扩展字段保持原生结构。`extra_body` 不能覆盖模型、提示词、参考图、mask、流式设置、认证或 Gemini 响应模态；已有工具参数须放在顶层。默认不发送 `response_format`；需要该字段的兼容模型可以显式放在 `extra_body` 中。

### 通过生成接口进行参考图编辑

部分 OpenAI 兼容模型的图生图使用生成接口，而非标准 edits 接口。两种 JSON 编辑格式请求同一个 `POST /v1/images/generations`，仅参考图字段的层级不同：

| 选项 | 参考图在最终 HTTP JSON 请求体中的位置 |
| --- | --- |
| 生成接口 · JSON image | `{"image":["data:image/png;base64,..."]}` |
| 生成接口 · JSON extra_body.image | `{"extra_body":{"image":["data:image/png;base64,..."]}}` |

按接入地址的实际接口选择，不能只根据模型名称判断。[Agnes 官方 Image 2.1 Flash 文档](https://agnes-ai.com/en/docs/agnes-image-21-flash)的图生图 curl 示例使用嵌套 `extra_body.image`，但同页参数表又列出顶层 `image`，文档本身存在不一致；[DMXAPI 的 Agnes 文档](https://doc.dmxapi.cn/agnes-image-21-flash-i2i.html)也明确要求嵌套字段。因此保留两种显式格式，分别兼容实际请求结构。

当前用户反馈 New API 中的 Agnes 配置使用顶层 `image` 已成功，应继续使用该格式。仓库内 `reference/new-api-main` 的 OpenAI 转发路径明确保留顶层 `image`；非透传路径重新序列化请求时，未知的 `extra_body` 可能被丢弃。这是基于参考代码的解释，不能据此认定已部署网关升级过、会自动转换嵌套字段，或所有 New API 渠道行为相同。`503 no available server` 来自上游服务，可能涉及接口路由或渠道容量，具体以 New API / 上游日志为准。

直连 Agnes 可配置 `provider=openai`、`endpoint_url=https://apihub.agnes-ai.com/v1`、`model_name=agnes-image-2.1-flash`，使用 Agnes API Key，编辑格式按官方 curl 示例选嵌套 `extra_body.image`，默认参数设置 `size`（例如 `1K`）。适配器覆盖该请求格式；尚未进行真实直连验收，不能保证具体账号或上游服务的可用性。

选择任一 JSON 编辑格式并保存后，继续调用原来的 `edit_image` 工具，调用参数中的 `images` 不变。服务验证并读取参考图，将真实图片字节转为带 MIME 的 data URL，避免上游访问 `localhost` 链接。JSON 编辑格式不支持 mask；服务会在调用上游前拒绝，工具 schema 和网页同时隐藏该参数。

multipart `/v1/images/edits` 是兼容协议，不限定只能连接 OpenAI 官方服务。[New API 插件调用指南](https://docs.newapi.pro/zh/docs/plugins/usage)允许支持 `openai_image` 的插件接管该接口，编辑可使用 JSON 或 multipart；具体模型、参考图字段和 mask 能力取决于渠道或插件实现。

如果渠道要求请求体中的原生 `extra_body.response_format`，由于 MCP 的 `extra_body` 是平铺扩展入口，应这样填写默认参数（是否需要 `size` 及可用值由渠道决定）：

```json
{
  "size": "1K",
  "extra_body": {
    "extra_body": {"response_format": "url"}
  }
}
```

上述 JSON 编辑请求的最终结构为 `{"model":"已配置模型","prompt":"调用提示词","n":1,"size":"1K","extra_body":{"response_format":"url","image":["data:image/png;base64,..."]}}`。参考图由服务写入，不能在默认参数或扩展字段中覆盖。适配格式由用户显式配置；上游失败不会自动切换接口或再次生成。

### OpenRouter 图片协议

协议选择 `openrouter`，使用 OpenRouter API Key，地址可留空或填写官方地址，也可使用兼容中转网关及其路径前缀。文生图和参考图编辑都请求 `POST /api/v1/images`；模型目录为 `GET /api/v1/images/models`。接口契约以 [OpenRouter 官方图片文档](https://openrouter.ai/docs/guides/overview/multimodal/image-generation)为准。

配置示例：

```json
{
  "name": "OpenRouter 图片",
  "provider": "openrouter",
  "model_name": "google/gemini-3.1-flash-image",
  "endpoint_url": "https://openrouter.ai/api/v1",
  "api_key": "<OPENROUTER_API_KEY>",
  "operation": "both",
  "default_params": {
    "resolution": "2K",
    "aspect_ratio": "16:9",
    "extra_body": {"seed": 42}
  }
}
```

继续使用 `generate_image` 和 `edit_image`。编辑工具的 `images` 经现有图片格式、所有者、签名和下载校验后，编码为 data URL，构造 `input_references` 的 `image_url` 结构；不直接转发 localhost 或私有存储链接。OpenRouter 使用固定 JSON 图片协议，忽略仅供 OpenAI 兼容接口使用的 `edit_request_format`，不支持 mask。

两种工具提供 `n`（1–10，默认 1）、`size`、`quality`、`background`、`output_format`、`aspect_ratio` 和 `resolution`，不使用 Gemini 的 `image_size`。未设置的参数不发送，尺寸组合及参考图数量限制由上游模型校验；可通过 `extra_body` 传入 `seed`、`output_compression` 或 `provider` 路由及提供商扩展，不能覆盖模型、提示词、参考图或流式设置。采用同步非流式请求，不做自动接口切换或失败重试。

解析响应的 `data[].b64_json`、`media_type`、可选 `url` 及 `usage`，复用现有存储、配额、签名下载和 inline 限额。实际 MIME 仍由图片字节确认。共享图片存储支持 PNG、JPEG、WebP；显式请求 SVG 会在调用上游前拒绝。上游仍使用专用同步 Image API，本地统一转为异步任务管理；未增加 Chat Completions 图片模式。

## MCP 工具

按 `operation` 暴露 `generate_image`、`edit_image`，所有配置均暴露 `get_image_status`；支持编辑时附带 `upload_image`。在网关中工具名带服务前缀，例如 `imagegen_1__generate_image`。

内置工具默认描述使用英文，说明能力范围、参数用法、上传流程及返回结果。详情页的 `upload_image` 描述直接使用后端注册文案；`generate_image`、`edit_image` 和 `get_image_status` 可自定义描述或恢复默认后保存。已有配置中的旧内置默认描述自动使用新版文案，自定义描述保留。启动时同步已注册服务的工具缓存。

```json
{"prompt":"绘制一幅水彩山景","return_mode":"url","wait_seconds":10,"request_id":"drawing-001"}
```

```json
{
  "prompt":"保留人物，背景改为海边",
  "images":["https://example.com/reference.png"],
  "mask":"data:image/png;base64,...",
  "return_mode":"inline"
}
```

- `images` 支持 HTTP(S) URL、data URL、base64，1 到 16 张。客户端本地图片先调用 `upload_image`，执行其返回的上传命令，再把 `image_url` 传给 `edit_image`。网页也可上传参考图和 mask。Agent 无法执行上传命令时，管理员可在「图片管理」（`/admin/uploads`）手动上传 PNG/JPEG/WebP 并复制地址，作为 `images` 数组的一项交给 Agent；调用需使用上传账号的 API Key。
- OpenAI 支持可选 `n`（1 到 10，默认 1）、字符串 `size`、`quality`、`background`、`output_format`；multipart 编辑请求单图字段为 `image`，多图为 `image[]`；JSON 编辑格式始终使用图片数组。
- OpenRouter 支持上述图片数量及通用生成参数，另提供 `aspect_ratio`、`resolution`；图片输入使用专用 JSON 参考图结构，不暴露 mask。
- mask 必须是 PNG，尺寸与首图一致，透明区域用于重绘；需要标准 OpenAI multipart 编辑接口及支持该能力的模型。Gemini / JSON 生成接口编辑的 mask 在访问上游前拒绝。
- Gemini 支持 `aspect_ratio`、`image_size`，参考图通过 `inlineData` 输入；单次请求不模拟多张生成循环。

### 短等待与查询

生成参数新增 `wait_seconds`（整数 0–30，默认 10）和可选 `request_id`（1–128 字节）；它们只在本地使用，不能放入 `default_params` 或 `extra_body`，不会发送上游。相同用户、配置、`request_id` 和输入返回同一任务；输入不同则拒绝。草稿还核对协议、连接、模型、默认参数等有效配置。未提供 ID 时每次调用视为新提交。

查询同配置任务：

```json
{"task_id":"<生成返回的本地 UUID>","wait_seconds":0,"return_mode":"url"}
```

查询支持 `wait_seconds`（默认 0，上限 30）和 `return_mode`（`url` / `inline`）。保存失败且 `can_retry_save=true` 时，传 `retry_download=true` 仅重试已有生成结果的下载、存储。不得传任意上游 ID；工具校验用户与配置归属。配置移除 MCP 入口后，仍可通过网页和任务 API 查看已有任务。

### 任务状态与恢复

| 状态 | 含义 |
| --- | --- |
| `queued` | 已持久化，等待后台 worker |
| `running` | 正在等待同步上游返回 |
| `saving` | 已获得生成结果，正在下载和保存 |
| `completed` | 所有图片已保存 |
| `partial` | 部分图片保存成功，可按 `can_retry_save` 重试其余结果 |
| `failed` | 明确拒绝、校验或保存失败，具体见错误 |
| `submission_unknown` | 网络超时、非明确的服务器错误或生成中断，无法确认上游结果 |
| `canceled` / `interrupted` | 升级前同步任务的历史状态 |

先入库再提交，每实例最多 4 个后台任务，数据库租约每 10 秒续期、30 秒到期，领取和状态更新核对持有者。连接、模型和有效参数按现有凭证机制密封快照，配置更改、停用、删除不影响已受理任务；草稿任务也可恢复。已排队任务重启后继续执行；已获得输出的 `saving` 任务从私有结果文件恢复保存。

图片上游没有可查询的异步任务 ID，正在同步生成的请求无法在进程重启后接续。过期的 `running` 租约转为 `submission_unknown`，不会自动重新 POST；HTTP 4xx 等明确拒绝为失败；HTTP 503 返回明确的 `image queue is full, please retry later`（可带 request id）同样标记失败。启动时会修正此前误标为结果不确定的同类队列拒绝历史，保留任务 ID、错误、创建时间及耗时，不重新生成。网络/期限/其他一般 5xx 保守标为结果不确定。查询和重试保存均不重新生成，避免重复费用。

获得上游结果后先写入独立私有对象 `imagegen/tasks/<task_id>/<lease>.json`，随后逐张保存并记录输出序号。保存重试跳过已保存的图片，不增加任务、不重复记录结果。凭证在输出落盘后从快照清除；全部保存成功后清除连接参数快照及临时结果文件。部分保存失败保留无凭证快照和临时结果以供重试；删除任务时同步清理私有结果文件。如果私有结果文件本身写入失败，仍尽力保存本次内存中的图片，但不能承诺重启恢复或后续保存重试。

## 结果与存储

所有调用返回 MCP 文本和 `structuredContent`，包含任务状态、`artifact_status`、`poll_after_seconds`、`can_retry_save` 及错误。排队、生成、保存及短等待结束返回正常结果；图片保存成功后附带 `resource_link`，结果包含 `task_id`、协议、模型、链接、实际 MIME、字节数、宽高、失效时间、修订提示词及可用的上游文本/用量。生成和上传共用本地/S3 存储、按用户配额、内容去重、签名 URL 和自动清理。

`return_mode=inline` 会追加 MCP 原生 `image` 内容。序列化响应合计超过 `ImageGenInlineMaxBytes`（默认 1 MiB；0 使用默认值）时保留链接并给出说明。生成图片的有效期独立按 `ImageGenRetentionHours`（默认 24 小时）计算；上传图片与参考缓存仍使用 `UploadRetentionHours`。管理员修改保留时长时，已有且未清理的对应来源图片从修改时刻重新计时，原创建时间保持不变；新增图片仍从自身创建时间计时。设为 0 表示永久保存，`expires_at` 返回 `null`；从永久改为 24 小时时，已有图片的到期时间为修改时刻加 24 小时。读取、历史列表、配额及清理共用该有效期，重复保存相同时长不会再次延长。

`ImageGenHistoryDays` 独立控制图片任务历史，默认 30 天，0 表示永久保留。后台每分钟按任务结束时间分批清理超期历史，同时释放已过期且不再共享的输出及暂存结果；运行中、正在保存或持有有效租约的任务不会被清理。与视频一致，仍有有效生成图片的历史会继续保留，因此文件设为永久保存时，对应历史也保留。仅将历史设为永久不会阻止图片文件按自身规则过期。

`MaxImagesPerUser` 控制每用户已保存生成图片的数量，默认 0（不限），独立于 `MaxUploadsPerUser`。过期图片不占生成图片配额；保存时重新检查并发配额，保存失败可通过保存重试恢复，不重复生成。相同用户、来源和图片内容去重；不同来源使用独立资产记录、共享物理文件，互不延长保留时间。删除任务会释放不再被其他输出任务使用的生成图片和数量配额，上传图片、参考图片以及其他任务共享的图片保留。

自有存储参考图读取前校验签名、所有者、上传状态和有效期。外部参考图下载禁止私网，校验 DNS 和每次重定向并限制字节数。上游输出允许访问明确配置的网关同源地址；下载图片不携带模型 API Key。上游失败不会自动重新生成；部分图片保存失败会保留已成功的链接并报告失败项。

### 浏览器能访问图片，但服务提示地址被禁止

代理的 Fake-IP DNS 可能把公网图片域名解析成 `198.18.0.0/15` 内的合成地址。浏览器通过代理能够访问，但这个地址不能直接通过服务的公网校验。

下载器检测到域名的全部系统 DNS 结果为该地址段时，会通过 Cloudflare DNS over HTTPS 查询真实的 A / AAAA 记录（最多 8 秒，调用方更短期限优先），继续校验并固定真实公网 IP 后连接。混合返回 Fake-IP 和真实地址时，跳过合成地址并校验其余地址。只有域名会触发该流程，直接填写的 Fake-IP 地址仍被拒绝；私网解析和危险重定向也不会放行。DoH 仅发送图片域名，不发送图片路径、模型提示词或 API Key。

若运行环境无法访问公共 DoH，错误会提示调整 DNS。使用 Mihomo / Clash Fake-IP 时，可在代理配置的 `dns.fake-ip-filter` 中加入对应图片域名，使其返回真实地址。例如黑名单模式下，在现有列表追加 `platform-outputs.agnes-ai.space`；规则模式则添加 `DOMAIN,platform-outputs.agnes-ai.space,real-ip`。修改后重载代理配置并刷新 DNS 缓存。无需重新配置模型密钥或关闭地址校验。

## HTTP API

所有接口使用用户认证，路径前缀 `/api/v1/imagegen`：

| 方法 | 路径 | 用途 |
|---|---|---|
| GET / POST | `/` | 列表 / 创建 |
| GET / PUT / DELETE | `/:id` | 详情 / 更新 / 删除 |
| POST | `/models` | 获取模型列表（支持 `config_id` 复用已保存密钥） |
| POST | `/test` | 测试当前配置，传 `tool`、`arguments` 和可选 `config_id` |
| POST | `/:id/enable`、`/:id/disable` | 注册 / 移除虚拟服务 |
| GET | `/tasks` | 当前用户的图片任务历史；支持 `config_id`、`page`、`page_size`（默认 10，最多 100） |
| GET / DELETE | `/tasks/:task_id` | 任务详情 / 删除历史记录（排队、生成、保存中返回 409） |
| POST | `/tasks/:task_id/retry-download` | 重试已有输出的保存，不重新生成 |

历史列表支持 `status` 状态筛选：省略或 `all` 返回全部，可用 `completed/queued/running/saving/partial/failed/submission_unknown/canceled/interrupted`；筛选先于分页和总数统计，并与 `config_id`、用户归属条件同时生效。页面在刷新按钮左侧提供下拉框，默认“全部状态”，“成功”对应 `completed`；切换时回到第一页并清空勾选。

新配置默认不注册。启用、禁用、删除及已注册配置更新通过事务修改数据库，成功提交后更新虚拟注册表；重复启用幂等。禁用/删除清理分组关联，重启恢复处理器并更新工具缓存。

## 图片任务历史

配置列表展示“生成能力”列（文生图、参考图编辑、两者），下方提供全部图片任务历史；配置详情下方只展示该配置的任务。任务记录覆盖网页测试（含未保存的配置）、直接 MCP、智能执行及批量执行，区分生成 / 编辑、调用来源、模型、提示词、参考图数量、处理状态、耗时、图片结果与错误。详情支持图片预览、下载和复制链接，历史按用户隔离。

历史表格在“耗时”右侧展示“过期时间”：多图任务优先取可用图片中最早的到期时间，全部过期时保留接口仍能提供的最早到期时间；可用图片均无期限时显示“永久保存”。图片已清理且无到期信息时显示“图片已过期或删除”，尚无图片结果时显示“—”。

任务历史支持本页多选和全选可删除任务，选中后显示与市场管理一致的批量操作栏，可批量删除或取消选择；运行中任务禁止勾选。批量删除沿用单条历史删除接口及用户归属检查，最多并行处理 4 条，部分失败会提示数量并保留失败项选择。切换页码或配置时清空选择，任务重新运行时自动取消其选择。分页与市场管理统一：左侧显示总条数，右侧箭头按钮和当前页/总页数，每页 10 条；删除后及数据刷新时自动回到有效页。

网页测试立即显示后台任务，每 5 秒查询；列表、详情对排队、生成、保存状态自动刷新，列表提供保存失败重试入口。所有活跃任务禁止删除。恢复规则见上文，不会自动重新提交结果不确定的生成请求。删除配置不会删除历史。删除任务会释放未被其他输出任务引用的生成图片；共享生成图片、上传图片及参考图片保留，物理文件仅在没有任何资产引用后删除。

生成结果在共享图片存储中标记为 `imagegen`，不展示在管理员图片管理或用户上传列表中，仍计入存储配额并参与自动清理。同一用户主动上传相同内容后，该上传仍正常展示。历史读取时按当前保留时间和图片存储记录计算可用性、签名链接；已过期或已清理的图片展示为不可用，任务本身保留。

图生图任务在请求上游前记录已校验的参考图和可选 mask。详情的“输入图片”区域按输入顺序仅展示小缩略图，点击后在弹窗查看大图、尺寸、格式、大小、保留时间、完整链接及复制链接/查看原图入口，mask 单独标注；上游生成失败仍保留输入记录。自有上传复用当前用户的图片资产；外部 URL、data URL 和 base64 输入保存实际校验后的图片字节，生成可用的本地签名链接，保证历史预览对应本次提交的内容。参考图缓存标记为 `reference`，不额外出现在上传管理列表；原有手动上传仍可见。参考图遵循图片保留时间与配额，0 为永久保存，过期或已删除显示占位提示；删除历史不立即删除共享图片。

公开历史只展示参考图元数据；数据库另有私有任务快照，用于后台执行和恢复。参考图只记录资产 ID，不在数据库保存参考图 base64 或旧签名链接；查询按用户归属重新生成链接。更新前的图片任务只记录参考图数量，无法可靠补回原图，详情提示未保存参考图。

## 验证

`go test ./...` 覆盖协议请求、模拟网关模型、真实 API/MCP 路由、用户隔离、注册事务和恢复、分组工具禁用、直接/智能/批量执行、mask、异步受理、客户端断开、幂等、并发租约、排队及保存阶段恢复、结果不确定、保存重试不重复生成、超时、内联降级、部分保存失败、下载限制以及本地/模拟 S3 保存。已有上传回归测试继续运行。前端验证使用 `npm run build:check`。

2026-10-17 异步改造验证记录：

- 相关后端包测试通过：`internal/imagegentask`、图片协议、共享存储与参考图、视频协议回归、`service`、`controller`、`router`、`model`。
- 本地模拟网关验证客户端短等待取消后继续完成、同请求 ID 并发去重、多实例租约、草稿快照恢复、生成中断结果不确定、400/429/503/超时分类、保存失败及部分保存重试、结果序号去重、公开 MCP 查询和原分组权限。
- SQLite 旧多图历史升级与重复迁移通过；MySQL/PostgreSQL 未提供专用 DSN，实际迁移尚未执行。
- 前端 `npm run build:check`、任务历史及视频参数脚本测试通过。没有调用真实付费图片网关；CherryStudio 的实际 Agent 轮询仍需部署后刷新工具列表验收。

模拟网关测试验证 GPT Image、Qwen、Seedream、OpenAI 图片接口上的 Imagen 及 Gemini 的请求格式，并覆盖 `seed`、`watermark`、`prompt_extend`。真实 New API 账号及具体上游模型的在线验收需要可用网关、授权密钥和模型额度。

设计参考：[writingmate/imagegen-mcp](https://github.com/writingmate/imagegen-mcp)、[OpenAI Images API](https://developers.openai.com/api/reference/resources/images/methods/generate)。上游保持同步非流式，本地异步封装；Replicate、任意 HTTP 模板、上游原生异步图片协议不在本次范围内。
