# 内置视频生成 MCP

> 更新日期：2026-10-16。使用说明与实际实现保持同步；验收结果见 [视频生成验收记录](./VIDEO-GENERATION-ACCEPTANCE.md)。

视频生成沿用图片生成的管理方式：创建配置，测试后启用，再将自动注册的内置 MCP 服务加入允许访问的分组。入口为 `/videogen`。新建配置默认禁用。

配置列表采用表格展示名称、协议、模型、生成能力和启用状态，状态为“已启用/已禁用”，操作列提供启用、禁用、详情和删除。搜索框即时筛选名称、协议或模型；移动端使用与图片生成一致的紧凑列表，删除位于更多菜单。任务历史同样使用表格，展示模型、任务 ID、提示词摘要、生成进度、保存状态和创建时间；详情弹窗完整展示提示词并保留换行，长内容可以滚动阅读，同时提供视频播放、下载及完整错误信息。进行中的任务不提供删除操作。

生成失败的记录提供“重试生成”，打开窗口后可修正提示词、参考图和 JSON 参数，再确认重新提交**当前任务**。任务 ID、创建时间及历史记录数量保持不变，原记录更新为本次参数和最新状态，`retry_count` 增加一次。使用所选已保存配置当前的连接、模型和默认参数；原任务是草稿或配置已删除时可选择其他配置，记录的配置关联也随之更新。调用参数快照供下次重试恢复；旧记录仅能恢复提示词，需要补充参数或重新上传参考图。生成重试可能产生上游费用；保存失败继续使用“重试保存”，提交结果不确定和进行中任务不展示生成重试。

## 快速使用

1. 在管理员设置中确认存储后端和 `ServerAddress`；后者用于生成的视频下载链接，应为客户端可访问的应用地址。New API 的 JSON 图生若需将 base64 图片保存为链接，该地址还需能被上游访问；OpenRouter 可直接发送经校验的图片 data URL。
2. 打开 `/videogen/create`，选择协议并填写 API Key 即可获取模型列表；基础地址可留空使用当前协议官方地址，无需预先填写名称或模型。模型弹窗默认搜索 `video`，可修改或清空搜索词，也可以直接手填模型 ID；搜索 Veo、Hailuo 时可改用 `veo`、`hailuo`，它们的模型 ID 不一定含有 `video`。再选择文生、图生或两者。
3. 在测试区选择文生或图生，输入提示词；图生上传单张参考图。草稿可以直接测试，无需先保存或注册。页面每 5 秒查询进度，保存成功后显示播放器、下载和复制链接；保存失败可点击“重试保存”。
4. 保存配置并点击“启用”，将自动注册的 `videogen_<id>` 服务加入分组，确认分组工具已启用，再将分组绑定到 MCP 客户端使用的 API Key。
5. 调用生成工具取得本地 `task_id`，按返回的 `poll_after_seconds` 调用状态工具，直到 `artifact_status=ready` 并出现 `videos` 链接。

支持文生和单图图生，不包含厂商原生接口、Responses、Remix、编辑或延长。`openai_video` 对接参考 New API 网关中的 Sora、Google/Vertex Veo、通义万相、豆包、可灵、海螺、即梦、Vidu 和 xAI；`openrouter_video` 对接 OpenRouter 的统一视频 API，覆盖 `heygen/heygen-video-1`、`minimax/hailuo-3-max`、`google/veo-3.1-lite` 等模型。协议测试通过不等于这些模型均已完成真实生成验收。

## 配置

- 协议：默认 `openai_video`（New API / OpenAI 视频格式），另可选 `openrouter_video`。模型名称允许手填，不根据名称自动切换协议。New API 网关即使代理了上述带厂商前缀的模型，仍按网关实际要求选择协议。
- 地址：可留空，OpenAI 视频默认 `https://api.openai.com/v1`，OpenRouter 视频默认 `https://openrouter.ai/api/v1`；模型查询、测试和保存配置均使用此默认值。自定义地址兼容根地址、`/v1` 或 `/api/v1` 及反向代理前缀。OpenRouter 协议下 `https://openrouter.ai` 自动补为 `https://openrouter.ai/api/v1`；其他网关根地址仍补 `/v1`。使用代理时填写其实际 API 前缀，例如 `https://proxy.example.com/router/api/v1`，避免把 OpenRouter 的 `/api` 路径遗漏。
- 能力：文生、图生或两者；默认两者。
- 图片格式：New API 默认 multipart。通义万相和豆包插件不接收 multipart 图片文件，图生需选择 **JSON**；xAI 的复杂 metadata 也建议选择 JSON。OpenRouter 图生使用 JSON 首帧结构。
- New API 的 JSON 图生保留经过下载、图片格式和归属校验的 URL；base64 图片先保存到共享图片存储，再提供签名 URL。OpenRouter 的 URL 同样先校验，base64 / data URL 则直接编码进 JSON，不为提交参考图依赖 `ServerAddress`；最终视频的本地下载链接仍使用该设置。
- 单次上游请求默认超时 30 秒；后台跟踪默认 1 小时。跟踪到期会暂停查询，查询该任务可以继续跟踪，不会再次生成。
- 默认参数为 JSON 对象，New API 示例为 `{"seconds":8,"size":"1280x720","metadata":{"resolution":"720p"}}`，OpenRouter 示例见下文。默认参数、本次测试参数和重试参数均可留空，等同于 `{}`；填写时必须是完整的 JSON 对象，不接受数组、`null` 或其他类型。格式错误会指出对应输入框，阻止提交。时长、分辨率与其他参数的可用组合由具体模型校验。
- 密钥使用现有 `common.Encrypt`/`CRYPTO_SECRET` 机制私有保存；未设置 `CRYPTO_SECRET` 时该机制是 base64 回退，并非加密。接口仅返回 `has_api_key`，密钥留空更新会保留原值。

配置和任务的凭证写入不记录 GORM SQL。已有任务保存自己的连接快照，配置改动、取消注册或删除不会取消或改变已提交任务。视频保存成功或生成失败后清除任务凭证；删除配置会清除配置凭证。

任务同时保存提交协议；修改配置的协议不影响已受理任务。失败任务手动“重试生成”时采用当前所选配置的协议，仍复用原任务记录。升级前的任务按 `openai_video` 恢复。

详情页按图片生成页面布局展示基础配置、注册工具、测试生成及任务历史。注册工具按**已保存**的生成能力展示，生成工具和状态工具的描述可分别保存或恢复默认，保存后同步到内置服务；`upload_image` 仅在图生能力启用时展示，并标明“系统内置 · 不可修改”。新建页面先保存基础配置，再进入详情编辑工具描述。

## 上游协议与格式

### Agnes Video 2.5 Flash 的 `mode is required`

`agnes-video-2.5-flash` 的文生视频要求上游请求包含顶层 `mode: "text"`，输出档位使用 `size: "720P"`。模型的生成模式与页面的文生/图生工具选择是两层参数；工具选择不会自动设置厂商的 `mode`。参数定义见 [Agnes 官方文档](https://wiki.agnes-ai.com/zh-Hans/docs/agnes-video-25-flash)。

文生测试可在“本次测试参数（JSON）”中填写以下内容；文生专用配置也可以将它保存为默认参数：

```json
{
  "size": "720P",
  "extra_body": {
    "mode": "text",
    "aspect_ratio": "16:9"
  }
}
```

最小文生扩展参数 `{"extra_body":{"mode":"text"}}` 是合法 JSON。另一个参数框可以留空；旧版页面直接解析空文本会弹出 `Unexpected end of JSON input`，更新前端后已按 `{}` 处理。实际模型是否还要求尺寸等参数由网关决定。

Go 程序内嵌前端资源。部署此修复时，先构建前端，再重新编译并重启 Go 程序，最后强制刷新浏览器；仅更新 `web/dist` 或刷新页面不会改变旧程序内嵌的资源。

`extra_body` 会平铺到上游请求顶层，实际发送的是 `mode` 和 `aspect_ratio`；不要把 Agnes 的 `mode` 放入 `metadata`，也不要直接在工具参数顶层填写未声明的 `mode`。示例省略 `seconds`，由 Agnes 使用自己的默认时长；其官方时长参数为字符串，当前统一工具的 `seconds` 是数字，显式传时长前需验证所接网关的类型兼容。

这份参数用于文生；Agnes 图生使用 `keyframe` 或 `reference`，还要求 `first_frame`/`last_frame` 或 `images` 等字段，当前工具的统一 `input_reference` 不能据此视为已支持其原生图生。Agnes 官方推荐的查询路径是 `/agnesapi`，而本工具使用统一 `/v1/videos/:id` 及 `/content`；修正提交参数后，还需验证所接 New API 对查询和内容下载的转换。保持统一协议范围，不根据模型名称静默切换厂商接口。

HTTP 400 任务可修正参数后，通过页面“重试生成”更新原记录。MCP 的生成工具仍按原 `request_id` 去重；再次独立调用生成工具时使用新的 `request_id`，这属于新建任务。

### 统一 OpenAI 视频协议（`openai_video`）

| 请求 | 作用 |
| --- | --- |
| `POST /v1/videos` | 提交生成，接受返回中的 `id` 或 `task_id` |
| `GET /v1/videos/{id}` | 查询已有上游任务 |
| `GET /v1/videos/{id}/content` | 使用配置凭证获取视频并保存 |

文生请求使用 JSON。图生的 multipart 模式以 `input_reference` 字段上传真实图片字节，`metadata` 编码为 JSON 字符串；JSON 模式以 `input_reference` 提交经校验的图片 URL。单图输入不接受客户端本地路径；JSON 模式下，data URL 或 base64 会经图片存储转换为可供上游访问的 URL。

通义万相、豆包插件要求图片 URL，因此图生选择 JSON；xAI 的 multipart 解码器不解析 metadata，使用这类插件参数时选择 JSON。其他模型的时长、分辨率和参数组合由网关校验。配置格式必须明确选择，失败不会自动切换格式或重复提交。

### OpenRouter 视频协议（`openrouter_video`）

使用 OpenRouter API Key，协议选择 `openrouter_video`，地址可填写 `https://openrouter.ai/api/v1`。以下三个模型通过同一套异步视频协议使用，无需选择各厂商原生接口：[HeyGen Video](https://openrouter.ai/heygen/heygen-video-1)、[MiniMax H3 Max](https://openrouter.ai/minimax/hailuo-3-max)、[Google Veo 3.1 Lite](https://openrouter.ai/google/veo-3.1-lite)。

配置示例：

```json
{
  "name": "OpenRouter 视频",
  "protocol": "openrouter_video",
  "model_name": "google/veo-3.1-lite",
  "endpoint_url": "https://openrouter.ai/api/v1",
  "api_key": "<OPENROUTER_API_KEY>",
  "operation": "both",
  "image_request_format": "json",
  "default_params": {
    "seconds": 4,
    "extra_body": {
      "resolution": "720p",
      "aspect_ratio": "16:9",
      "generate_audio": false
    }
  }
}
```

`seconds` 转为上游整数 `duration`，不接受小数时长；也可不传 `seconds`，改用 `extra_body.duration`。两者不要同时填写。`size` 用于像素尺寸；`resolution`、`aspect_ratio`、`generate_audio`、`seed` 和 `provider.options` 等扩展放入 `extra_body`，例如仅替换模型 ID 后的默认参数可按模型文档调整：

| 模型 ID | 参数示例 |
| --- | --- |
| `heygen/heygen-video-1` | `{"seconds":5,"extra_body":{"resolution":"480p","aspect_ratio":"16:9"}}` |
| `minimax/hailuo-3-max` | `{"seconds":5,"extra_body":{"resolution":"480p","aspect_ratio":"16:9"}}` |
| `google/veo-3.1-lite` | `{"seconds":4,"extra_body":{"resolution":"720p","aspect_ratio":"16:9","generate_audio":false}}` |

这些是请求格式示例，未完成真实生成验收。可用时长、尺寸、音频和提供商参数组合以目标上游为准；项目模型目录仅返回 `id` / `name`，没有实现按目录动态限制参数或模型白名单。默认搜索词 `video` 可清空或改为 `veo`、`hailuo`。

非空 `metadata` 会在本地拒绝；它是 New API 插件参数的入口，不能用于 OpenRouter。OpenRouter 的提供商扩展应使用 `extra_body.provider.options.<提供商>.parameters`。`extra_body` 不能替换参考图字段或模型、提示词、凭证和任务控制参数，也不能填写 `previous_job_id`（含大小写变体）；本期不开放额外的多图、音视频参考、尾帧、编辑或延长能力。[官方提交参数](https://openrouter.ai/docs/api/api-reference/video-generation/submit-a-video-generation-request)

图生继续调用 `generate_video_from_image` 并传入单个 `image`；适配器在已有下载、格式和归属校验之后构造 `frame_images` 首帧。公开图片 URL 与 data URL 均可使用，不向上游发送客户端本地路径。OpenRouter 的官方图生示例使用 `frame_type: "first_frame"`，其官方仓库也说明图片可用 data URL。[图生流程](https://openrouter.ai/docs/cookbook/video-generation/image-to-video)、[官方图片编码说明](https://github.com/OpenRouterTeam/skills/blob/main/skills/openrouter-video/SKILL.md)

| 请求 | 作用 |
| --- | --- |
| `GET /api/v1/videos/models` | 专用视频模型目录 |
| `POST /api/v1/videos` | JSON 提交，接受 202 返回的 `id` 与状态 |
| `GET /api/v1/videos/{id}` | 按已持久化 ID 查询 |
| `GET /api/v1/videos/{id}/content?index=0` | 携带 Bearer 凭证下载第一个视频并保存 |

查询和下载根据配置地址及已保存 ID 构造固定路径，不跟随返回中的 `polling_url`、`unsigned_urls` 或签名 `urls`；完成响应没有这些可选链接时仍可尝试保存内容。`pending` 映射为排队，`in_progress` 为生成中，`completed` 为生成完成，`failed/cancelled/expired` 映射为生成失败。上游 `expired` 与已保存本地文件的 `artifact_status=expired` 是不同状态。[官方查询接口](https://openrouter.ai/docs/api/api-reference/video-generation/poll-video-generation-status)、[官方内容接口](https://openrouter.ai/docs/api/api-reference/video-generation/download-generated-video-content)

## MCP 工具

实际工具名称带服务前缀，例如 `videogen_1__generate_video`。

| 工具 | 必填输入 | 可选输入 |
| --- | --- | --- |
| `generate_video` | `prompt` | `seconds`、`size`、`extra_body`、`wait_seconds`、`request_id`；`metadata` 仅用于 `openai_video` |
| `generate_video_from_image` | `prompt`、`image` | 同文生；`image` 接受 HTTP(S) URL、data URL 或小型 base64 |
| `get_video_status` | 本地 `task_id` | `wait_seconds`、`retry_download` |
| `upload_image` | `local_path` | 上传单张参考图；仅启用图生能力时提供 |

```json
{
  "name": "videogen_1__generate_video",
  "arguments": {
    "prompt": "A slow camera pan over mountain scenery",
    "seconds": 4,
    "wait_seconds": 10,
    "request_id": "my-video-request-001"
  }
}
```

工具返回本地 `task_id`、生成状态、`artifact_status` 和建议查询间隔。进行中或等待结束不是错误；用 `get_video_status` 继续查询。生成工具的等待为 0–30 秒，默认 10 秒；状态工具默认不等待。相同用户、配置及 `request_id` 的相同输入复用任务，修改输入会被拒绝。

```json
{
  "name": "videogen_1__get_video_status",
  "arguments": {
    "task_id": "<生成工具返回的本地任务 ID>",
    "wait_seconds": 0
  }
}
```

| 状态字段 | 含义与后续操作 |
| --- | --- |
| `status=submitting/queued/in_progress/unknown` | 尚未确认生成完成；继续查询原任务 |
| `status=completed` | 上游生成完成；还需检查保存状态 |
| `artifact_status=pending/saving` | 等待或正在保存文件 |
| `artifact_status=ready` | 可播放下载，链接位于 `videos` |
| `artifact_status=failed` | 保存失败；重试保存，不重新生成 |
| `artifact_status=expired` | 已保存视频过期，历史记录仍可保留 |
| `tracking_paused=true` | 自动跟踪暂停；查询原任务可恢复，生成状态保持原值 |
| `status=failed/submission_unknown` | 生成失败或提交结果不明确；查看错误信息，不自动重提 |

`generate_video_from_image` 额外要求 `image`。本地文件先调用 `upload_image`，执行返回的上传命令，然后使用其 `image_url`。文生配置不暴露上传或图生工具。

图生调用示例（先选择图生或两者能力，URL 应指向有效图片）：

```json
{
  "name": "videogen_1__generate_video_from_image",
  "arguments": {
    "prompt": "保持画面主体，镜头缓慢向前移动",
    "image": "<upload_image 上传完成后返回的 image_url>",
    "wait_seconds": 0,
    "request_id": "my-image-video-001"
  }
}
```

Direct 模式使用上面的命名空间；Smart 模式先搜索、查看工具 schema，再通过 `mcp.execute` 使用 `videogen_1.generate_video` 等 tool_id。后续状态查询依赖前一次返回的 task_id，分步执行。生成工具应启用对应分组权限；取消注册后仍可在管理 API 或页面查询旧任务。

`openai_video` 的 `metadata` 承载网关插件参数，multipart 会将其序列化为 JSON 字符串；其他复杂扩展参数需使用 JSON 模式。`openrouter_video` 不接受非空 `metadata`，参数通过 `extra_body` 传入。两种协议都禁止扩展覆盖模型、提示词、参考图、鉴权或本地任务控制参数。

保存失败时，使用 `get_video_status` 的 `retry_download=true` 或页面的“重试保存”。它只获取已有视频，不再创建生成任务。提交超时、响应缺少任务 ID 或进程中断造成的 `submission_unknown` 不会自动重试生成，以免重复消耗上游额度。

提交返回 HTTP 503 且结构化错误码为 `video_queue_full` 时，网关明确拒绝受理，任务标为 `failed`（生成失败），队列恢复后可点击“重试生成”。系统不会自动重新提交，原请求 ID 仍复用原失败记录。普通 5xx、超时或无法解析的响应继续标为 `submission_unknown`；已取得上游 ID 后，查询中的临时 503 只延后查询，不改变生成状态。

New API 的任务代理可能使用外层 `fail_to_fetch_task` 等通用代码，将上游完整 JSON 放入 `message`、`error.message` 或字符串 `error`。此时也会解析显示出来的完整 JSON，识别其中确切的 `video_queue_full`；仅包含队列相关文字或截断 JSON 不据此判定失败。若重试后队列仍满，原记录再次显示生成失败。同一次重试请求 ID 重复发送只返回当前记录，不重新提交；不同输入复用同一重试 ID 会被拒绝。去重状态持久化，重启及多个实例并发也不会因同一次重试新增记录或重复生成。

启动时会修正旧版误标的队列拒绝记录：仅处理无上游 ID、未被其他实例领取、保存有完整 `503 + video_queue_full` JSON 错误的 `submission_unknown` 任务，改为 `failed` 并保留原错误和输入。已有上游 ID、响应不完整或仅包含队列相关文字的记录保持原状态。

## API 与结果

所有管理 API 使用现有用户 JWT，路径前缀为 `/api/v1`。

字段、请求示例与响应层级见 [API 文档 §13](./API.md#13-视频生成接口)；持久化字段见 [数据库设计](./DATABASE.md#220-videogen_configs---视频生成配置表)，部署要求见 [部署指南 §6](./DEPLOY.md#6-视频生成部署)。

| 接口 | 用途 |
| --- | --- |
| `GET/POST /videogen` | 配置列表与创建 |
| `GET/PUT/DELETE /videogen/:id` | 配置详情、更新与删除 |
| `POST /videogen/:id/enable`、`disable` | 启用与禁用，同时注册/取消注册内置服务 |
| `POST /videogen/models` | 模型目录，支持草稿字段及 `config_id` |
| `POST /videogen/test` | 草稿或已保存配置测试，附 `tool`、`arguments` |
| `GET /videogen/tasks` | 分页任务历史，可用 `config_id` 过滤 |
| `GET /videogen/tasks/:task_id` | 查看任务，恢复暂停的跟踪 |
| `POST /videogen/tasks/:task_id/retry-download` | 重试保存 |
| `GET /videogen/tasks/:task_id/retry-input` | 恢复失败任务输入，不返回连接或密钥 |
| `POST /videogen/tasks/:task_id/retry` | 修正参数后在原任务记录上重新提交生成 |
| `DELETE /videogen/tasks/:task_id` | 删除停止处理的任务及视频；进行中返回 409 |

完成的 MCP 结果包含 `structuredContent.videos` 和 `resource_link`，不内联视频二进制。视频通过独立签名的 `/v/:sid?s=…` 读取，支持 GET、HEAD、Range 和浏览器拖动播放；到期返回 410。任务历史、任务详情及配置测试结果的下载按钮附加 `download=1`，服务端以附件返回并提供 `.mp4` / `.webm` 文件名，点击直接下载；正常播放链接保持原样。生成状态与保存状态独立，文件过期不改变上游生成成功的事实。

新增设置：`VideoGenMaxBytes=268435456`、`VideoGenRetentionHours=24`、`VideoGenHistoryDays=30`、`MaxVideosPerUser=0`（不限）。文件上限和保留设置与图片存储独立，物理后端共用 local/S3。每实例最多 4 个处理任务；数据库租约和版本条件防止多实例重复领取。清理每分钟执行，保留未到期视频的历史；中断进程留下的临时文件在安全宽限期后清理。

## 验证

```powershell
go test ./...
cd web
npm run build:check
```

`TestNewAPIGatewayPluginCompatibility` 在存在 `reference/new-api-main` 和 Node 时执行十类插件的真实协议解码器，校验 30 组请求及格式限制；不产生上游生成费用。

`TestVideoGenMigrations` 默认检查 SQLite，包含已有图片配置保持、新表重复迁移及唯一性检查。MySQL/PostgreSQL 使用以下环境变量指向**空的专用测试数据库**；测试会创建并删除测试表，不要使用应用数据库：

- `VIDEOGEN_TEST_MYSQL_DSN`
- `VIDEOGEN_TEST_POSTGRES_DSN`

S3 的 Range 行为通过协议测试服务器验证；生产验收还应连接实际 S3 兼容后端。真实网关验收必须使用具备视频权限的模型和密钥，模型目录访问成功不代表视频生成权限；每次生成可能产生上游费用。

真实测试可设置 `VIDEOGEN_SMOKE_ENDPOINT`、`VIDEOGEN_SMOKE_API_KEY`、`VIDEOGEN_SMOKE_TEXT_MODEL`、`VIDEOGEN_SMOKE_IMAGE_MODEL`，然后运行 `go test ./internal/mcp/videogen -run TestLiveGatewayVideoSmoke -v -timeout 35m`。该测试使用独立临时数据库，提交一次文生、一次图生并保存样片至 `tmp/videogen-smoke`。可选 `VIDEOGEN_SMOKE_SECONDS`、`VIDEOGEN_SMOKE_SIZE`、`VIDEOGEN_SMOKE_IMAGE_FORMAT`；JSON 图生另需设置可被上游访问的 `VIDEOGEN_SMOKE_IMAGE_URL`。未配置时测试跳过，不读取应用凭证或产生生成费用。

实际运行结果、测试覆盖、浏览器模拟验收和待完成的真实环境检查见 [视频生成验收记录](./VIDEO-GENERATION-ACCEPTANCE.md)。
