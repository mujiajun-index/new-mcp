# MCP 服务多秘钥(单/多模式,随机/轮询策略)

> 2026-09 V1。仅 `streamable-http` / `SSE` 两类 HTTP 传输;stdio 不支持(env 在进程
> spawn 时固定,无法按请求轮换)。单秘钥服务保持 `config.headers` 现状,零改动、零兼容代码。
> 市场条目级池(一份池全局轮换)已于 V1.1 落地(§6)。

## 1. 背景与范围

上游按 key 限频/计额度时,单 key 是吞吐瓶颈;key 失效则服务整体不可用。为 MCP
服务引入秘钥池:

| 能力 | 说明 |
|------|------|
| 模式 | 单秘钥(现状)/ 多秘钥 |
| 策略 | 随机(启用集合均匀抽取)/ 轮询(游标推进回绕) |
| 更新 | 追加(去重、保留既有状态)/ 替换(整池重排、状态清零) |
| 管理 | 池列表(掩码)、单把启/禁/删、批量(全部启用/删除已禁用)、模式与策略切换 |
| 熔断 | 上游 401/403 自动禁用该 key(落库 + 系统日志),会话不断、下一请求自动换 key |
| 归因 | `mcp_call_logs.key_index` 记录每次调用实际使用的池内序号 |
| 边界 | 服务级池(自有服务)与条目级池(市场 instant HTTP 条目,一份池全局轮换)并存;克隆多秘钥源服务整拷为条目池 |

## 2. 数据模型

### 2.1 `mcp_service_keys` — 秘钥池(新表)

| 列 | 说明 |
|----|----|
| `service_id` + `sort_order` | 联合唯一索引 `idx_svc_key_pos`;序号从 **1** 起 = 轮询次序 = 日志 `key_index`(0 保留给「未使用多秘钥」) |
| `value` | TEXT,明文(与市场 `config_template` 内凭证同等 at-rest 待遇);单把 ≤8KB,池上限 100 把 |
| `status` | 挂**行**上:1=启用 / 2=手动禁用 / 3=自动禁用(行级状态,删 key 序号重排不串) |
| `disabled_reason` / `disabled_at` | 自动禁用原因(当前固定 `upstream 401/403`)与时间 |

`replace` 更新走事务:删全量 + 从 1 重排插入;`append` 只追加去重、既有行与状态不动。

### 2.2 `mcp_services.auth_config` — 模式与注入头(零新增列)

`mcp_services` 行宽贴近 MySQL 65535 上限,不新增 varchar 列;复用原"只写不读"的
`auth_config` JSON 承载:

```json
{ "key_mode": "random",   // ""/缺省 = 单秘钥;random | polling
  "header_name": "X-API-Key" }
```

- 多秘钥运行时只读 `header_name`(单一读路径,不按 AuthType 二次推导);
  `api_key`→`X-API-Key`、`bearer`→`Authorization`(值加 `Bearer ` 前缀)、
  `custom`→创建/切换时从现有 `config.headers` 键里显式选定。
- 多秘钥模式下 `config.headers` 不再放该认证头(动态值在静态头之后 Set,同名覆盖,
  防御两处并存)。

### 2.3 `mcp_call_logs.key_index` — 调用归因

多秘钥调用写入实际使用秘钥的池内 `sort_order`;0 = 单秘钥/不适用。

## 3. 运行时设计

```
gateway CallTool ──> SDKAdapter.CallWithMeta
                      │ Pick() 一次(策略选 key)──> ctx 携带 {index, value}
                      ▼
                   sess.CallTool(ctx)
                      │ go-sdk 把 ctx 传给每个上游 HTTP 请求
                      ▼
              headerRoundTripper.RoundTrip
                ctx 有选择 → 全程同一把(POST/GET 一致,并发调用互不串)
                ctx 无选择 → POST 现选(轮换);GET/DELETE 沿用最近一次 POST 值
                响应 401/403 → OnAuthFailure(本次实际用的 index)
```

- **KeySelector**(`internal/mcp/bridge/key_selector.go`):每属主一个,进程级注册表
  `bridge.KeySelectors` 惰性构建(读池快照进内存),与会话生命周期解耦——池编辑/
  模式切换后 `Invalidate` / `InvalidateItem` + 踢会话,下次建连按新池重建;轮询游标为内存态。
  **市场引用行分流**:`Get(svc)` 遇 `source=marketplace` 的行改按条目 ID 构建选择器
  (一份池对全部安装用户全局共享,坏 key 一次禁光;session_pool 两个调用点零改动),
  熔断落库与日志文案按属主类型分流(服务池 / 条目池)。
- **归因精度**:`CallWithMeta`(transport 包 `MetaCaller` 接口)在发起 `tools/call` 前
  Pick 一次放进 ctx,同一次调用的所有 HTTP 请求(POST + GET 流)全程同一把 key;
  资源/提示有同款 `ReadResourceWithMeta` / `GetPromptWithMeta`(`ResourceMetaCaller` /
  `PromptMetaCaller` 可选接口)。网关(tools/call、原生 resources/read、prompts/get、
  智能模式 mcp.execute / mcp.execute_batch / mcp.read)与服务详情页手动测试(工具/
  资源/提示)均经这些变体把 `KeyIndex` 写进 `mcp_call_logs.key_index`(本地失败未调
  上游时为 0);RoundTripper 内 401/403 观察用的是本次请求局部变量,熔断归属天然准确。
- **故障转移**:MCP 鉴权按 HTTP 请求生效、协议无会话-密钥绑定 → 会话不断,下一请求
  自动换 key;`GetOrConnect` 建连失败(initialize 即 401,坏 key 已被熔断)时对多秘钥
  服务换 key 立即重试一次。全部禁用时返回明确错误并记系统日志,**不禁用服务行**
  (服务是用户自己的,提示修复更合适)。
- **变更联动**:池更新/模式切换/策略切换/AuthType 变更(bearer 前缀是选择器输入)
  → `KeySelectors.Invalidate` + `SessionPool.Remove` + 异步重连预热;删除服务时
  连带清池并失效选择器。

## 4. 管理 API

所有权靠 `model.GetServiceByID(userID, serviceID)` 隐式校验;非 HTTP 传输或市场托管
服务返回错误。详细请求/响应见 `docs/API.md` §4。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/services/:id/keys` | 池列表(掩码值,永不回明文)+ key_mode + header_name + 启用数 |
| PUT | `/api/v1/services/:id/keys` | `{mode: append\|replace, values:[]}` 批量更新,返回新增/跳过数 |
| PUT | `/api/v1/services/:id/keys/:keyID` | 启/禁单把(重启用清 disabled_reason/at) |
| DELETE | `/api/v1/services/:id/keys/:keyID` | 删除单把 |
| POST | `/api/v1/services/:id/keys/batch` | `{action: enable_all\|delete_disabled}` |
| PUT | `/api/v1/services/:id/keys/config` | 模式/策略切换;单→多收编现有认证头值为首把,多→单首选启用 key 写回 headers 并清池 |

创建入口:`POST /api/v1/services` 带 `key_mode` + `auth_keys[]` 一步建池。

条目级池的管理端点同构(AdminAuth 把关,属主为市场条目;见 `docs/API.md` §4):

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/admin/marketplace/:id/keys` | 条目池列表(掩码值)+ key_mode + header_name + 启用数 |
| PUT | `/api/v1/admin/marketplace/:id/keys` | `{mode: append\|replace, values:[]}` 批量更新 |
| PUT | `/api/v1/admin/marketplace/:id/keys/:keyID` | 启/禁单把 |
| DELETE | `/api/v1/admin/marketplace/:id/keys/:keyID` | 删除单把 |
| POST | `/api/v1/admin/marketplace/:id/keys/batch` | `{action: enable_all\|delete_disabled}` |
| PUT | `/api/v1/admin/marketplace/:id/keys/config` | 模式/策略切换;单→多收编模板认证头为首把,多→单首选启用 key 写回模板并清池 |

仅 `category=instant` 的 sse/streamable-http 条目可管理;DTO 与服务级完全复用(auth_type
由服务端按 header_name/模板 headers 反推)。池/模式变更后 `InvalidateItem` +
`SessionPool.RemoveByMarketplaceItem`(踢该条目全部引用会话,按需重连)。

## 5. 前端

- **创建页**第 3 步选秘钥模式(单 / 多·随机 / 多·轮询,stdio 隐藏)+ 批量粘贴;
  第 4 步逐把循环调既有 `testConnection` 展示每把通过/失败。
- **详情页**「秘钥管理」卡片(完整管理能力):池表格、
  添加对话框(追加/替换单选,替换二次确认)、批量操作、全部禁用红色横幅、自动禁用
  原因悬停;多秘钥模式下认证区锁定并指向该卡片。卡片为通用组件
  (`web/src/components/service-keys-card.tsx`,注入 `KeysApi` 端点适配):服务详情页与
  **市场管理详情页**(instant HTTP 条目,一份池全局轮换)复用同一交互;市场页上游
  配置卡在多秘钥态同样锁定认证区,概览头部加同款多秘钥徽章。
- **列表/详情**「多秘钥·随机/轮询」琥珀徽章;**日志页** `#N` 秘钥索引徽章。
- **凭证掩码**(与市场管理详情同策):服务详情响应的 `config.headers/env` 凭证值只出
  首尾掩码(`maskConfigCredentials`),明文不出服务端;编辑保存时未改动的掩码值经
  `mergeMaskedCredentials` 回填还原,改动过的按新值入库。原「连接配置」原始 JSON
  转储卡已移除。

## 6. 市场边界:条目级池(V1.1 已落地)

市场条目有自己的秘钥池(`marketplace_item_keys`,与 `mcp_service_keys` 同构,属主为
条目):**一份池对全部安装用户全局轮换**,坏 key 一次禁光,`key_index` 归因与服务级
同一口径。选择器按条目 ID 构建(`Get(svc)` 对市场引用行自动分流),挂上后网关各调用
路径的轮换、熔断、归因零改动生效。

- **克隆上架**:`CloneFromService` 遇多秘钥源服务**整拷**为条目池——模板 headers 剥掉
  认证头(值只存池),`{key_mode, header_name, bearer}` 写入 `marketplace_items.auth_config`,
  池整拷(状态重置为启用)+ 系统日志;源池为空时拒绝克隆。
- **存量单秘钥条目**:管理详情页秘钥卡片里单→多,模板认证头收编为首把 key(同服务级
  交互);多→单首选启用 key 写回模板。
- 池编辑/模式切换/删条目:`InvalidateItem` + `RemoveByMarketplaceItem` 踢该条目全部
  引用会话;删条目同事务硬删池行(注册表不残留明文快照)。

## 7. 性能分析

### 7.1 设计立场:高频可变状态不持久化

轮询游标与内存禁用集是**每次选 key(≈每次上游请求)都要读写的状态**;若把游标
持久化到数据库,选 key 就会退化为每请求一次 DB 读 + DB 写,并发下再叠加行锁竞争。
new-mcp 的游标因此从不落库——DB 只存秘钥池本身与状态的权威值(管理操作和 401/403
熔断时写入),运行期高频路径全部走进程内存。

### 7.2 每请求成本

选 key 的全部开销(与 DB/Redis 无关):

| 路径 | 操作 | 成本 |
|------|------|------|
| 每次逻辑调用(`CallWithMeta`) | `Pick()`:1 次互斥锁 + 池扫描(≤100 行,通常几把) + 1 次 `context.WithValue` | 数十 ns 量级,无分配以外的系统调用 |
| 每个上游 HTTP 请求(RoundTripper) | 1 次 ctx 取值 + 1 次 `Header.Set`(静态头注入的 `req.Clone` 原本就有) | 可忽略 |
| 并发归因 | key 选择经 ctx 传递,RoundTripper 无额外同步 | 可忽略 |
| 401/403 熔断(仅失败路径) | 1 次 `SetKeyStatus` UPDATE + 1 条系统日志,**锁外**执行,不阻塞其他 Pick | 失败请求本身已注定失败,不拖累成功路径 |
| 池编辑/模式切换(管理操作) | `Invalidate` + 踢会话 + 异步重连(重建时读一次池) | 管理频率,无关热路径 |

锁竞争评估:`Pick` 临界区是 ≤100 元素的内存扫描,单次持锁百 ns 以内;即便万级 QPS
全部打到同一服务,锁排队也远小于一次上游 HTTP 往返(毫秒级)——瓶颈永远在上游而非
选 key。`headerRoundTripper.lastMu` 只覆盖两个标量赋值,同理。

**结论:轮询/随机策略在任何部署形态下都是纯内存操作,无需引入 Redis 或缓存层。**

### 7.3 有意付出的代价与边界

| 代价 | 评估 |
|------|------|
| 重启后轮询游标归零 | 只影响轮换起点,keys 与禁用状态都在 DB,重载即恢复;无害 |
| 单实例假设 | `KeySelectors` 与 `SessionPool` 同为进程内存态——多副本部署本就不被现有会话架构支持,多秘钥未引入新约束。注意:自动禁用虽落库,其他副本(若未来部署)的选择器快照不会自动感知,需 Invalidate/重启 |
| 池快照与 DB 短暂不一致 | 编辑池后已建会话仍持旧快照,直至 Invalidate + 踢会话重建(管理路径已联动);秘钥池不是高频变更数据 |
| random 模式每次 Pick 分配启用切片 | ≤100 元素的小分配,纳秒级;不为它引入复杂度 |

### 7.4 已修的隐患

删除服务时 `KeySelectors.Invalidate` 原先缺失,已删多秘钥服务的选择器快照(含明文
key)会残留注册表——内存泄漏 + 明文滞留。已在 `service.Delete` 补失效。

## 8. 测试与验证

- `internal/mcp/transport/dynamic_auth_integration_test.go`:真实 go-sdk 握手端到端——
  坏 key initialize 401 熔断 → 换 key 建连成功 → `CallWithMeta` 归因 `KeyIndex=2`,
  附带资源/提示读取的 WithMeta 归因断言。
- `internal/mcp/bridge/key_selector_test.go`:轮询跳禁用/回绕、随机跳禁用、空池报错、
  Bearer 前缀、条目属主(kind=item)池行为。
- `service/marketplace_keys_test.go`:条目池升/降级(收编写回、bearer 位、模板剥头/
  写回)、守卫(无认证头/换头/source/stdio)、追加去重/替换重排/批量、克隆整拷
  (含空池拒绝、单秘钥源不变)。
- `service/service_config_mask_test.go`:详情响应凭证掩码(url 明文、headers 掩码、
  Bearer 方案保留)、保存回填(未改动还原明文、改动存新值)。
- `go build ./...` / `go vet ./...` / `go test ./...`、`npx tsc --noEmit`、
  `npm run build` 全绿(本机缺 gcc,`-race` 不可用)。

## 9. 二期边界(未做)

- stdio env 多秘钥(重启进程轮换)。
- 结果内容级熔断规则(insufficient quota 等文本)。
- 日志按 `key_index` 筛选聚合(每把 key 的调用量/成功率统计)。
