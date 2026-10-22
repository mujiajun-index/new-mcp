# 内置音频生成

音频生成沿用图片和视频模块的虚拟服务、分组、客户端 API Key 与文件存储。每个配置包含三个可选模型槽位，各槽位选择配置内的连接，连接通过 `base_url` 和 `api_key` 访问兼容网关。配置新建后默认未注册，需要在列表启用并加入分组。

## 配置示例

```json
{
  "name": "Audio studio",
  "connections": [
    {"key": "router", "protocol": "openrouter", "base_url": "https://openrouter.ai/api/v1", "api_key": "YOUR_KEY"}
  ],
  "models": [
    {"kind": "speech", "enabled": true, "connection_key": "router", "model": "google/gemini-3.8-flash-tts", "default_params": {"voice": "Kore"}},
    {"kind": "audio", "enabled": true, "connection_key": "router", "model": "openai/gpt-audio", "default_params": {"voice": "alloy"}},
    {"kind": "music", "enabled": true, "connection_key": "router", "model": "google/lyria-3-pro-preview", "default_params": {}}
  ]
}
```

`POST /api/v1/audiogen` 新建，`PUT /api/v1/audiogen/:id` 更新。连接和模型数组提交时替换对应集合；同名连接留空密钥保留原密钥，修改 URL 时必须重新填写密钥。返回值仅包含 `has_api_key`，不返回密钥。保存的 `default_params` 返回 JSON 字符串，提交时使用 JSON 对象。

协议在每个模型连接中选择，槽位绑定连接后自动适配，不再单独选择协议。服务端根据连接协议重新计算实际适配器，保存、草稿测试和模型发现使用同一规则。

| 连接协议 | 配音 | 语音创作 | 音乐 |
| --- | --- | --- | --- |
| `openai` | `openai_speech` | `openai_chat_audio` | 不支持 |
| `openrouter` | `openrouter_speech` | `openrouter_chat_audio` | `openrouter_chat_audio` |
| `gemini` | `gemini_generate_content` | 不支持 | `gemini_generate_content` |
| `minimax` | 不支持 | 不支持 | `minimax_music` |

连接协议仅提供 OpenAI、Gemini、OpenRouter、MiniMax Music 四项，按此顺序显示。槽位和模型发现请求只提供连接标识与用途，实际适配器统一由服务端计算。不支持当前用途的连接不能绑定；切换渠道时会更新适配器和输入能力，不自动替换模型 ID。

Base URL 可留空，默认地址如下。新建连接或切换默认渠道时自动填写对应地址；已填写的自定义网关地址不会被覆盖。清空地址后，保存、获取模型和测试均使用对应默认地址。

| 连接协议 | 默认 Base URL |
| --- | --- |
| OpenAI | `https://api.openai.com/v1` |
| Gemini | `https://generativelanguage.googleapis.com/v1beta` |
| OpenRouter | `https://openrouter.ai/api/v1` |
| MiniMax Music | `https://api.minimax.io/v1` |

每种用途最多一个模型。需要同时保留 Lyria、MiniMax 两套音乐模型时创建两个配置。保存至少一个完整槽位，注册中的配置至少启用一个槽位。更新工具集时保留服务 ID，已停用的槽位会在调用时再次校验。

可配置 `capabilities`：`input_modalities`、`output_modalities`、`max_speakers`、`lyrics`、`instrumental`、`lyrics_optimizer`。已知模型提供预设，模型发现返回可用的模态信息；自定义别名可手动调整到协议支持的范围。未知模型不会仅因为配置在音乐槽位就被默认认定支持图片。

测试模型的音色输入支持下拉选择和手动输入。根据模型及适配器提供 OpenAI TTS、Gemini TTS 和 Chat Audio 的内置音色；`tts-1`、`tts-1-hd` 使用对应的音色子集。模型发现返回 `supported_voices` 时优先使用渠道提供的列表，未识别的模型仍可手填音色 ID。音色建议不限制自定义值，也不会写入配置默认参数；更换测试模型或连接会清空已选音色。

## 工具

| 工具 | 输入 |
| --- | --- |
| `generate_speech` | `text`；或 `turns: [{role,text,instructions?}]` 与 `speakers: [{role,voice}]` |
| `generate_audio` | `prompt` 和／或 `audios: [reference]` |
| `generate_music` | `prompt`、`images: [reference]`、`lyrics`、`is_instrumental`、`lyrics_optimizer`，按模型能力开放 |
| `get_audio_status` | `task_id`、可选 `wait_seconds`、`retry_download`、`return_mode` |
| `upload_image` | 支持图片音乐模型的 `local_path` 上传准备 |
| `upload_audio` | 支持录音输入模型的 `local_path` 上传准备 |

工具在分组中使用现有命名规则，例如 `audiogen_1__generate_music`。三种生成工具只在对应槽位启用时发布，两个上传工具仅在需要时发布。

```json
{"prompt":"根据这张图片创作舒缓的钢琴配乐","images":["https://example.com/reference.jpg"],"is_instrumental":true,"request_id":"music-001","wait_seconds":10}
```

```json
{"prompt":"听录音并用中文回答其中的问题","audios":["https://example.com/recording.wav"],"voice":"alloy","request_id":"reply-001","wait_seconds":10}
```

本地文件先调用上传工具，然后用 shell 执行返回的 `upload_command`，最后将 `image_url` 放入 `images` 或将 `audio_url` 放入 `audios`。服务器不读取客户端磁盘。引用支持本系统签名地址、HTTP(S) URL、Data URL 和小型 Base64；音频输入支持 WAV、MP3。服务器检查归属、有效期、实际格式、大小和外部地址，再将实际字节转换给上游。

生成默认等待 10 秒，状态查询默认立即返回，等待范围 0–30 秒。等待超时不是生成失败，后续通过 `get_audio_status` 查询。`return_mode=url` 返回保存后的 `resource_link`；`inline` 在总量不超过上限时附带 MCP 原生 `audio` 内容，否则返回链接和提示。

同一用户、配置、用途的 `request_id` 用于幂等；相同 ID 不同输入会报错。媒体输入指纹使用字节摘要和顺序。开始一项新的创作应使用新的 `request_id`。

## 协议

| 协议 | API | 说明 |
| --- | --- | --- |
| `openai_speech` | `/v1/audio/speech` | 原生配音 |
| `openai_chat_audio` | `/v1/chat/completions` | 文字／音频输入，语音回答 |
| `openai_realtime` | `/v1/realtime` WebSocket | 文字输入，收集音频增量并检查最终完成状态 |
| `openrouter_speech` | `/api/v1/audio/speech` | 原生配音，按模型支持多人 |
| `openrouter_chat_audio` | `/api/v1/chat/completions` | 聚合音频流；支持模型提供的录音或图片输入 |
| `gemini_generate_content` | `/v1beta/models/{model}:generateContent` | Gemini TTS、Lyria |
| `gemini_interactions` | `/v1beta/interactions` | Gemini TTS、Lyria |
| `minimax_music` | `/v1/music_generation` | MiniMax Music 3 标准音乐 API |

URL 允许自定义前缀；版本段不会重复追加。模型 ID 按用户配置发送，不自动替换。`extra_body` 接收非结构性上游参数；模型、媒体、认证、输入结构、流式和本地任务控制不能覆盖。Realtime 首期使用已声明的文字、音色和表达指令参数。

OpenRouter 将 TTS 的 `Speech` 和聊天／音乐的 `Audio` 分成两类模型目录。连接选择 `openrouter` 后，配音自动使用 `openrouter_speech`，获取模型查询 `/models?output_modalities=speech`；语音创作和音乐自动使用 `openrouter_chat_audio`，查询 `/models?output_modalities=audio`。两类模型的生成接口不同，列表按实际适配器匹配，不将 Speech 模型当作 Chat Audio 模型发送。没有模态元数据的自定义模型别名仍可选择或手动输入。

Gemini TTS 经 OpenRouter 时默认请求 `response_format: "pcm"`，按已知的 24 kHz、16 位单声道参数封装为 WAV 保存和播放；响应头中的采样率、声道数优先。公共参数 `format: "wav"` 也使用此流程，其他压缩格式在提交前报错。其他模型的默认格式不受影响。

GPT Audio 经 Chat Audio 传输 `input_audio: {data,format}`，不将音频 URL 直接传给 OpenRouter。Lyria 参考图分别转成 Gemini `inlineData`、Interactions 图片内容块或 OpenRouter `image_url`。歌词和纯音乐要求在 Lyria 中转成文字指令，音乐请求不注入配音音色。

MiniMax 音乐槽位使用 `music-3.0` 等标准模型 ID。无歌词的非纯音乐默认启用 `lyrics_optimizer`；显式关闭时需要歌词。`is_instrumental` 与指定歌词互斥。使用非流式请求，默认接收 Hex，也解析 URL 结果，并检查 `base_resp.status_code`。没有自部署、翻唱或参考音频适配。

## 任务和文件

任务生成状态为 `queued`、`running`、`completed`、`failed`、`submission_unknown`，文件保存状态单独记录。任务入队时保存连接和媒体输入快照，客户端断开、配置修改或删除不影响已接受任务。

完整结果持久暂存后再正式保存；暂存完成标记和摘要用于重启时排除部分写入。生成期间进程退出、连接中断且无法确认结果时，不自动重复付费生成。文件保存失败时，只重试读取已暂存的结果；仅在暂存仍可恢复时提供保存重试。明确失败的生成允许人工重试，保持原任务 ID、创建时间与用途，累加重试次数。

输入与输出音频记录在独立资产中。签名 `/a/:sid` 支持 GET、HEAD、Range；PUT 使用独立签名，仅可写入未过期的待上传槽位。读取签名不能上传，已上传的槽位不能覆盖。参考图继续使用现有图片资产。历史引用资产 ID，不保存临时链接作为长期访问依据；素材过期后重试需要重新上传。

原生格式直接保存。只有采样率、位深及声道数已知的 PCM 才封装 WAV，不使用 FFmpeg。生成语音的转写、回答文字、输入录音转写和歌词分别保存；未返回的输入录音转写不会从回答文本推断。

管理接口位于 `/api/v1/audiogen`：配置 CRUD、`/:id/enable`、`/:id/disable`、`/models`、`/test`、`/uploads`、`/tasks`；任务支持详情、`/retry-input`、`/retry`、`/retry-download`、删除和 `/tasks/batch-delete`。模型发现失败时可手动输入模型 ID，测试不要求先保存配置。

| 设置 | 默认值 |
| --- | --- |
| 槽位 `timeout_seconds` | 600 秒 |
| `AudioGenConcurrency` | 4 |
| `AudioGenInputMaxBytes` | 25 MiB，总输入音频 |
| `AudioGenMaxBytes` | 128 MiB，总输出音频 |
| `AudioGenInlineMaxBytes` | 1 MiB；0 关闭内联 |
| `AudioGenRetentionHours` | 24；0 永久 |
| `AudioGenHistoryDays` | 30；0 永久 |

## 迁移和验证

2026-10-21 核对官方公告：OpenAI 的 `tts-1`、`tts-1-hd` 以及列出的两个 `gpt-4o-mini-tts` 快照将于 2027-01-06 下线，推荐迁移到 `gpt-realtime-2.1-mini`；`gpt-audio` 列有 2027-01-20 下线及迁移到 `gpt-audio-1.5` 的安排。具体模型迁移不等于所有兼容 Speech 端点关闭。MiniMax 官方公告音乐 API 不再向新用户开放，现有付费用户可以继续使用；实际权限错误由连接服务返回。

协议与生命周期使用模拟上游进行自动测试，包括 HTTP/SSE/WebSocket、图片和录音输入、业务失败、幂等、跨用户／分组隔离、快照、重启恢复、保存重试、上传签名和 Range。真实服务须使用用户自己的 URL、密钥和可用模型进行付费联调，自动测试不代表真实上游已验证。

参考：[blacktop/mcp-tts](https://github.com/blacktop/mcp-tts)、[OpenAI 音频接口](https://developers.openai.com/api/docs/guides/audio-chat-completions)、[OpenAI 弃用公告](https://developers.openai.com/api/docs/deprecations)、[OpenRouter 音频](https://openrouter.ai/docs/guides/overview/multimodal/audio)、[Gemini TTS](https://ai.google.dev/gemini-api/docs/speech-generation)、[Gemini 音乐](https://ai.google.dev/gemini-api/docs/music-generation)、[MiniMax 音乐](https://platform.minimax.io/docs/api-reference/music-generation)。
