# 图片生成 MCP 模拟验收结果

验收日期：2026-10-10。按交付约定，本次使用隔离数据库、模拟网关和测试密钥；未调用真实 New API 上游或消耗模型额度。

## 自动验证

| 检查 | 结果 |
|---|---|
| `go test ./... -timeout 240s` | 通过，包含现有上传回归测试 |
| `npm run typecheck`（`web`） | 通过 |
| `npm run build:check`（`web`，包含 TypeScript 检查和生产构建） | 通过 |
| `git diff --check HEAD` | 通过 |

Go 验收覆盖：

- OpenAI / Gemini 地址规范化、认证、参数省略和覆盖、保护字段、模型列表不按名称过滤。
- GPT Image、Qwen 生成/编辑、Seedream、OpenAI 图片接口上的 Imagen 和 Gemini 原生的模拟请求；网关参数 `seed`、`watermark`、`prompt_extend`。
- 单图 / 多图 multipart、PNG mask 和尺寸校验；Gemini 文本与图片混合候选响应；URL / base64 输出；失败不重试生成。
- 配置用户隔离、密钥不回显、幂等注册、事务回滚、缓存更新、重启恢复、禁用和删除关联清理。
- 实际 Gin / MCP 路由上的直接、智能、批量调用及分组工具权限；显式期限和取消上下文。
- 本地和模拟 S3 保存、去重、配额、签名读取、上传状态和有效期、清理、外部下载限制、同源网关输出下载不携带密钥。
- 实际图片 MIME / 尺寸元数据、原生内联内容及超限降级、部分保存失败保留成功链接、服务测试的 0 / 自定义超时规则。

对应测试文件：`internal/mcp/imagegen/*_test.go`、`internal/imageasset/store_test.go`、`service/imagegen_test.go`、`router/imagegen_integration_test.go`。

## 网页手工验收

使用隔离的本地实例完成：

1. 创建 OpenAI 兼容配置，预览最终请求地址；获取并选择模拟 Qwen 模型，显式测试生成图片。
2. 保存配置后确认默认未注册；进入详情启用，确认服务关联和工具描述。
3. API Key 输入框不回显已保存密钥，仍可使用保存的密钥测试。
4. 上传参考图并测试编辑，确认编辑请求地址、图片预览、元数据及签名链接。
5. 复制链接并核对剪贴板；下载得到 PNG 文件。
6. 创建分组并添加图片服务，确认来源标识和 `generate_image`、`edit_image`、`upload_image` 三个聚合工具。

MCP 生成、编辑和下载的完整调用链另由路由集成测试自动验证。网页验收临时服务在交付前停止；测试数据与业务数据库隔离。

## 验收边界

这些结果证明接口格式、注册流程、存储和应用调用链在模拟网关下可用。真实 New API 部署中的模型可用性、上游参数限制、计费及出图质量仍需用目标网关和授权模型单独联调。首版为同步非流式调用。

使用方式见 [图片生成 MCP 说明](IMAGE-GENERATION.md)。

## Fake-IP 下载问题修复验证（2026-10-10）

- 当前环境将 `platform-outputs.agnes-ai.space` 解析为 `198.18.0.133`，复现了浏览器可访问而下载器拦截的原因。
- 修复后，用用户已有的输出 URL 验证实际下载成功：PNG、1,370,270 字节、1024 × 1024。仅下载已生成结果，没有重新调用模型。
- 新增回归覆盖 Fake-IP 域名回退、公网 / 私网地址隔离、字面 IP 拒绝、混合解析拒绝、回退结果再次校验、DoH 响应格式和大小限制、超时取消及重定向校验。
- 回归命令：`go test ./internal/mcp/imagegen ./internal/imageasset ./service ./router -timeout 180s`。
