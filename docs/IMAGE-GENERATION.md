# 内置图片生成 MCP

在「图片生成」页面创建配置，每条配置绑定一个模型。先获取模型或手动填写模型名，再显式测试出图，保存并启用，最后在分组中添加 `imagegen_<配置 ID>` 服务。获取模型只读取模型列表，不生成图片；测试出图会调用上游。

## 配置

- `provider`：`openai` 或 `gemini`。OpenAI 兼容模式可连接 New API 图片接口；Gemini 使用原生 `generateContent`。
- `endpoint_url`：网关根地址、反向代理路径前缀或带 `/v1`、`/v1beta` 的基础地址均可；页面预览最终地址。
- `operation`：`generate`、`edit`、`both`（默认）。能力由配置和上游决定，不根据模型名猜测。
- `timeout_seconds`：默认 180；0 仅跟随调用方期限。MCP 智能/批量执行显式的 `timeout_ms`、请求取消及更短的调用方期限优先。
- `default_params`：JSON 对象。调用参数覆盖默认值，`extra_body` 深度合并；未填写的可选参数不发送。
- API Key 只保存在独立配置中。列表、详情和虚拟服务配置不回显密钥；编辑时留空保留原密钥。

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

## MCP 工具

按 `operation` 暴露 `generate_image`、`edit_image`；支持编辑时附带 `upload_image`。在网关中工具名带服务前缀，例如 `imagegen_1__generate_image`。

内置工具默认描述使用英文，说明能力范围、参数用法、上传流程及返回结果。详情页的 `upload_image` 描述直接使用后端注册文案；`generate_image` 和 `edit_image` 可自定义描述或恢复默认后保存。已有配置中的旧内置默认描述自动使用新版文案，自定义描述保留。启动时同步已注册服务的工具缓存。

```json
{"prompt":"绘制一幅水彩山景","return_mode":"url"}
```

```json
{
  "prompt":"保留人物，背景改为海边",
  "images":["https://example.com/reference.png"],
  "mask":"data:image/png;base64,...",
  "return_mode":"inline"
}
```

- `images` 支持 HTTP(S) URL、data URL、base64，1 到 16 张。客户端本地图片先调用 `upload_image`，执行其返回的上传命令，再把 `image_url` 传给 `edit_image`。网页也可上传参考图和 mask。
- OpenAI 支持可选 `n`（1 到 10，默认 1）、字符串 `size`、`quality`、`background`、`output_format`；编辑请求单图字段为 `image`，多图为 `image[]`。
- mask 必须是 PNG，尺寸与首图一致，透明区域用于重绘；具体模型须支持此能力。Gemini mask 在访问上游前拒绝。
- Gemini 支持 `aspect_ratio`、`image_size`，参考图通过 `inlineData` 输入；单次请求不模拟多张生成循环。

## 结果与存储

默认返回 MCP 文本、图片 `resource_link` 和 `structuredContent`，包含协议、模型、链接、实际 MIME、字节数、宽高、失效时间、修订提示词及可用的上游文本/用量。生成和上传共用本地/S3 存储、按用户配额、内容去重、签名 URL 和自动清理。

`return_mode=inline` 会追加 MCP 原生 `image` 内容。序列化响应合计超过 `ImageGenInlineMaxBytes`（默认 1 MiB；0 使用默认值）时保留链接并给出说明。图片有效期按 `UploadRetentionHours` 计算；管理员更改保留规则后，实际有效期随规则变化。

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

新配置默认不注册。启用、禁用、删除及已注册配置更新通过事务修改数据库，成功提交后更新虚拟注册表；重复启用幂等。禁用/删除清理分组关联，重启恢复处理器并更新工具缓存。

## 验证

`go test ./...` 覆盖协议请求、模拟网关模型、真实 API/MCP 路由、用户隔离、注册事务和恢复、分组工具禁用、直接/智能/批量执行、mask、超时取消、内联降级、部分保存失败、下载限制以及本地/模拟 S3 保存。已有上传回归测试继续运行。前端验证使用 `npm run build:check`。

模拟网关测试验证 GPT Image、Qwen、Seedream、OpenAI 图片接口上的 Imagen 及 Gemini 的请求格式，并覆盖 `seed`、`watermark`、`prompt_extend`。真实 New API 账号及具体上游模型的在线验收需要可用网关、授权密钥和模型额度。

设计参考：[writingmate/imagegen-mcp](https://github.com/writingmate/imagegen-mcp)、[OpenAI Images API](https://developers.openai.com/api/reference/resources/images/methods/generate)。首版同步非流式；Replicate、任意 HTTP 模板和独立异步任务不在本次范围内。
