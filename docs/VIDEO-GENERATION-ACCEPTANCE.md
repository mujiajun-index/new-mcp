# 视频生成 MCP 验收记录

验收日期：2026-10-12。环境为 Go 1.26.3 / Windows amd64、SQLite 3.41.2。已完成自动测试和隔离环境的页面模拟流程；真实视频生成、实际 S3 桶及 MySQL/PostgreSQL 验收尚未完成。使用说明见 [内置视频生成 MCP](./VIDEO-GENERATION.md)。

## 2026-10-13 详情提示词全文展示

- 移除任务详情提示词的三行截断，保留换行和空行，长连续字符自动换行。详情沿用弹窗内的垂直滚动，列表继续展示摘要。
- 本地模拟使用八段文本、长连续字符和全文结束标记验证：桌面和 390px 窄屏均无行数限制，提示词渲染高度等于其滚动高度；详情无横向溢出，滚动后可访问最后一段。
- `npm run build:check` 和 Go 编译通过，文档已更新，新程序为 `tmp/videogen-prompt-fix/newmcp.exe`。此次仅调整页面显示，没有提交生成任务或修改业务记录。

## 2026-10-13 重试复用原任务记录

当前重试行为及验收以本节为准，下方条目保留此前实现的验证记录。

- “重试生成”改为在原任务行重新提交，保留 `task_id`、创建时间、初始来源和原提交幂等映射；更新当前配置、模型、提示词和参数，重置上游 ID、错误、进度及跟踪状态，不增加任务历史数量。
- 新增私有 `retry_requests` 请求哈希映射和 `retry_count`。SQLite 验证旧行新增字段与重复迁移，MySQL/PostgreSQL 真实迁移仍因专用 DSN 未设置而跳过。旧版 `retry_of` 字段兼容保留。
- 相关 Go 测试覆盖同任务 ID、历史数量不变、参数验证失败不改原记录、图生恢复、原提交与重试去重、重启后的去重、较早重试请求重放、两个实例的同请求/不同请求并发、租约与旧处理结果不能覆盖新提交。队列再次拒绝仍更新原记录为失败。
- `go test ./internal/mcp/videogen ./model ./service ./router -count=1`、新增并发测试、相关 `go vet` 和前端 `npm run build:check` 通过。新程序为 `tmp/videogen-same-task-fix/newmcp.exe`。
- 模拟页面验证第 2 页的原失败任务 `qa-failed-005` 原位更新为排队中，分页仍为 2/2，提示词更新，详情显示同一任务 ID；日志仅一次重试 POST。此次未向真实网关生成，未中断业务服务或修改业务任务，需重启应用使用本次版本。

## 2026-10-13 前端对齐补充

- 配置状态、按钮及成功提示统一为“已启用/已禁用”和“启用/禁用”，与图片生成一致。
- 任务历史改为表格，使用模拟任务验证分页、生成进度、保存状态、重试保存、继续跟踪和详情弹窗；进行中任务不展示删除按钮。详情中保留播放器、下载和错误信息。
- 详情页基础配置、内置服务提示、注册工具和测试区域沿用图片生成布局。验证文生能力展示生成和状态两个可编辑工具；文生/图生展示三个可编辑工具及只读上传工具。描述保存请求仅包含对应字段，恢复默认需保存生效。
- `npm run build:check`、相关前端文件格式检查和 `git diff --check` 通过。1280px 桌面及 390px 移动视口没有页面横向溢出，任务表格在自身容器中横向滚动。
- 此次页面验证使用本地模拟 API 和最小 MP4 文件头，只检查播放器节点与链接展示，没有执行真实生成或可播放视频验收；下方真实网关待验收项继续有效。

## 2026-10-13 Agnes 参数诊断

- 只读核对本地任务：`agnes-video-2.5-flash` 的文生提交返回 HTTP 400 `mode is required`，所关联配置的默认参数为空。
- 核对 [Agnes 官方参数文档](https://wiki.agnes-ai.com/zh-Hans/docs/agnes-video-25-flash)：文生需要顶层 `mode=text`，Flash 的输出档位为 `720P`。在本工具中通过 `extra_body.mode` 传入，由适配器平铺至请求顶层。
- 使用本地 HTTP 协议样例复现空参数时的 400，确认带 `size=720P`、`extra_body.mode=text` 和 `extra_body.aspect_ratio=16:9` 的请求通过样例校验。未修改应用配置，未向真实网关重新提交任务。
- 此次确认的是参数错误原因及本地序列化；Agnes 的真实生成、统一查询/内容接口和原生图生仍未完成验收。

## 2026-10-13 失败记录重试补充

- 失败生成的列表和详情新增可编辑的重试窗口，使用当前已保存配置创建新任务，保留原记录及 `retry_of` 关联。旧记录缺少输入时提示补充参数或参考图，草稿任务可选择已保存配置。
- 后端覆盖原输入恢复、当前默认参数合并、同一次重试去重、变更输入拒绝、跨用户/配置拒绝、非失败和提交未知任务拒绝、旧图生补充参考图及私有快照不公开。保存失败仍只重试下载。
- SQLite 验证旧视频记录增加请求快照和重试关联字段、重复启动迁移以及原数据保持。MySQL/PostgreSQL 真实迁移仍待专用 DSN。
- 视频任务、路由、服务及模型相关 Go 测试通过；前端类型检查和生产构建通过。模拟 UI 验证“重试生成”入口、参数编辑、提交后新任务及移动端布局，未向真实网关重新生成视频。

## 2026-10-13 JSON 参数空输入修复

- 在修复前的构建中复现：默认参数留空、本次测试参数填写 `{"extra_body":{"mode":"text"}}`，点击生成弹出 `Unexpected end of JSON input`，尚未发送测试请求。
- 默认、测试、生成重试三个参数框的空文本和纯空白统一按 `{}` 处理。不完整 JSON、数组、`null` 及标量仍阻止提交，使用中英文提示指出对应字段。
- `npm run test:videogen-params` 的 8 项回归通过，覆盖空输入、用户提供的 mode 参数、无效格式/类型、字段定位及嵌套参数原样保留。`npm run build:check`、相关文件格式检查及 `git diff --check` 通过。
- 本地模拟页面验证空默认参数保存、两个参数框分别留空时的测试提交、空重试参数创建新任务，以及无效 JSON 未发出请求。未提交真实网关生成，未修改业务配置或任务。
- 使用最新前端执行 `go build -buildvcs=false -o tmp/videogen-json-fix/newmcp.exe ./cmd/server/` 通过，并确认程序包含修复后的参数提示资源。旧运行程序仍需替换、重启后才会提供新页面。

## 2026-10-13 队列已满分类修复

- 只读核对用户提供的请求 ID `2026101306172161127757pXOiR4rA`：任务无上游 ID，收到完整 HTTP 503 `video_queue_full`，旧规则将其误标为 `submission_unknown`。
- 提交时解析顶层或 `error.code` 中的结构化错误码；仅 `503 + video_queue_full` 确认队列拒绝并标为 `failed`。普通 503、504、传输超时、空响应和截断响应仍为提交未知，查询中的临时 503 保留已受理任务。
- 覆盖显式重试创建新任务、重复原请求 ID 复用失败记录、无自动重提，以及启动时修正 103 条跨批次历史样例。历史修正重复执行不改变修订号，保留停止时间、原错误和输入；已有上游 ID、租约持有者、不完整错误或仅队列文字的记录不变。
- `go test ./internal/mcp/videogen ./internal/mcp/virtual ./service ./router -count=1` 通过（virtual 无独立测试，调用链由路由测试覆盖）。此次未提交真实网关、未直接修改业务数据库，历史记录将在使用修复版本启动时修正。
- `go vet ./internal/mcp/videogen ./service ./router` 及 Go 编译通过，生成修复程序 `tmp/videogen-queue-fix/newmcp.exe`；包含前一轮的空 JSON 参数修复和最新前端资源。

## 2026-10-13 New API 错误包装与重复拒绝补充

- 只读核对本地重试记录和应用日志：原记录 `aee93c79-a144-456e-b3b3-83809c3e8432` 为失败；一次重试 POST 创建关联记录 `286189b9-f8ae-44fb-b116-449bcd46f9ba`，再次收到队列已满，误标为提交未知。此次点击没有重复 POST。
- 参考网关的任务代理会以通用外层代码封装上游正文到 `message`。用本地同形响应复现原规则的误判，错误显示与上述记录一致；业务记录不包含原始外层响应，此次没有重复提交真实上游来抓取响应。
- 补充兼容完整内层 JSON 错误，覆盖顶层 message、error.message、字符串 error 和数字外层代码。普通文字、截断 JSON、普通 5xx 及传输超时仍保留提交未知。
- 增加生成重试再次收到包装队列拒绝的回归：只创建一个新任务，仍为 `failed`，重复同一次重试 ID 返回该记录。历史修正保留重试关联与输入。
- `go test ./internal/mcp/videogen ./service ./router -count=1`、对应 `go vet`、`git diff --check` 和 Go 编译通过。更新程序为 `tmp/videogen-retry-fix/newmcp.exe`；当前业务服务未被中断，需重启以应用此次错误包装修复。

## 自动验证

| 检查 | 结果 |
| --- | --- |
| `go test ./... -count=1` | 通过，包含图片生成回归；最后的错误信息与文件名调整另运行相关测试并通过 |
| `go vet ./internal/mcp/videogen ./internal/videoasset ./service ./router ./model` | 通过 |
| `npm run build:check`（web） | TypeScript 检查及生产构建通过 |
| `git diff --check HEAD` | 通过 |
| 十类 New API 插件协议解码 | 30 组请求满足预期，含格式拒绝用例，不调用真实视频模型 |
| SQLite 新建、升级及重复迁移 | 通过；保留已有图片配置，检查幂等键和可空唯一短 ID |
| S3 寻址读取 | 协议模拟服务器验证 Range，未连接真实桶 |
| MySQL / PostgreSQL 迁移 | 专用 DSN 未设置，子测试跳过 |
| 真实网关文生、图生 | 验收参数未设置，未执行 |
| ESLint | 项目缺少配置文件，未完成 |

## 测试覆盖与文件

| 范围 | 覆盖 | 测试文件 |
| --- | --- | --- |
| 客户端 | 地址拼接、参数保护、JSON/multipart、参考图、metadata、错误脱敏、id/task_id | [client_test.go](../internal/mcp/videogen/client_test.go) |
| 任务处理 | 断开后继续、request_id 去重、重启恢复、跟踪暂停、临时查询错误、未知提交、租约、删除、保存重试及到期清理 | [manager_test.go](../internal/mcp/videogen/manager_test.go) |
| 视频资产 | 保存上限、MP4 保存、HTML 拒绝、对象读取及独立签名域 | [store_test.go](../internal/videoasset/store_test.go) |
| S3 读取 | 可寻址对象读取和 Range 请求 | [video_seek_test.go](../internal/storage/video_seek_test.go) |
| 数据库 | SQLite 与可选 MySQL/PostgreSQL 的迁移、重复启动和唯一索引 | [videogen_migration_test.go](../model/videogen_migration_test.go) |
| API/MCP 完整流程 | 配置/草稿测试、注册、参考图、直接/Smart/批量调用、分组权限、用户隔离、签名与 GET/HEAD/Range | [videogen_integration_test.go](../router/videogen_integration_test.go) |

保存重试测试核对上游提交次数，确保只重新下载已有生成结果。生成与保存分别记录状态，临时查询错误不丢失任务；配置停用或删除不改变已提交任务的连接快照。

## 网关协议检查

`TestNewAPIGatewayPluginCompatibility` 将客户端生成的文生 JSON、图生 JSON 和图生 multipart 请求交给 `reference/new-api-main` 中各插件的真实解码器，共 10 × 3 组。检查由 [check-video-gateway-protocols.mjs](../scripts/check-video-gateway-protocols.mjs) 执行，参考目录或 Node 不存在时会跳过。

| 插件 | 协议检查结论 |
| --- | --- |
| `sora` | 三种代表请求满足预期 |
| `google` | 三种代表请求满足预期 |
| `vertex-ai` | 三种代表请求满足预期 |
| `alibaba` | 文生/图生 JSON 可解码；图生 multipart 图片文件按预期被拒绝，使用 JSON URL |
| `doubao` | 文生/图生 JSON 可解码；图生 multipart 图片文件按预期被拒绝，使用 JSON URL |
| `kling` | 三种代表请求满足预期 |
| `hailuo` | 三种代表请求满足预期 |
| `jimeng` | 三种代表请求满足预期 |
| `vidu` | 三种代表请求满足预期 |
| `xai` | 三种代表请求满足预期；multipart 不解析 metadata，使用此类参数时选择 JSON |

这里验证的是协议结构和格式约束。未据此验证生产网关的视频权限、排队时长、模型费用、播放质量或厂商的全部参数组合；模型名称与时长、分辨率仍由目标网关决定。

## 浏览器模拟验收

使用独立 SQLite 数据库、本地协议模拟网关和测试密钥完成：

1. 搜索、选择模型，保存配置并确认默认未注册。
2. 注册虚拟 MCP 服务，检查来源和工具描述。
3. 使用参考图测试，确认异步状态刷新及任务历史展示。
4. 检查保存成功后的播放器、下载入口和链接。
5. 在 1280px 桌面及 390px 窄屏检查布局，无横向溢出，控制台无错误或警告。

模拟视频用于验证控件与接口，未进行真实视频播放质量验收。网页临时服务已停止，测试数据库与业务数据库隔离。MCP 分组、上传、查询和下载调用链由路由集成测试验证。

## 真实环境待验收

| 项目 | 实际模型 / 环境 | 结果 |
| --- | --- | --- |
| 一次文生视频 | 未配置授权测试模型 | 待执行 |
| 一次单图图生 | 未配置授权测试模型及格式 | 待执行 |
| 实际 S3 后端 | 未连接测试桶 | 待执行 |
| MySQL / PostgreSQL | 未配置专用空测试库 DSN；Docker 守护进程未运行 | 待执行 |

现有网关的只读模型目录查询返回 403，后续网络重试未能连接；没有提交真实生成任务。完成验收后应补录网关版本、实际模型 ID、图生格式、生成时长、文件 MIME/大小及播放/Range 结果，不能将跳过的测试记为通过。

MySQL/PostgreSQL 迁移测试通过 `VIDEOGEN_TEST_MYSQL_DSN`、`VIDEOGEN_TEST_POSTGRES_DSN` 指向空的专用测试数据库；测试会创建和删除表，不使用业务数据库。真实生成测试使用 `VIDEOGEN_SMOKE_ENDPOINT`、`VIDEOGEN_SMOKE_API_KEY`、`VIDEOGEN_SMOKE_TEXT_MODEL`、`VIDEOGEN_SMOKE_IMAGE_MODEL`，会提交两笔可能收费的任务；JSON 图生另需可供上游访问的 `VIDEOGEN_SMOKE_IMAGE_URL`。执行命令与可选参数见[验证说明](./VIDEO-GENERATION.md#验证)。
