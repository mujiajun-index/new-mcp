# NewMCP 商业化模块设计文档

> 版本: V1.8 | 状态: 草案 | 更新日期: 2026-07-17
> 关联文档: [PRD.md](./PRD.md) V3 商业化 · [ARCHITECTURE.md](./ARCHITECTURE.md) §7.3 · [DATABASE.md](./DATABASE.md) · [API.md](./API.md)
>
> **变更摘要**:
> - V1.1:① 纯按次固定单价(取消 per_token);② 预扣→确认/退款;③ 服务市场平台托管。
> - V1.2:④ 余额不足拒绝本次调用 + 低额度提醒,不禁用 Key。
> - V1.3:⑤ 市场服务 3 级定价(工具>服务>全局)+ 批量设价;⑥ 用户自有服务免费 + `UserOwnedServicesEnabled` 开关;⑦ 管理员用户管理内调额。
> - V1.4:⑧ 市场上架 = 克隆(深拷贝,凭证保留并提示替换)/ 手动;市场与自有服务隔离。
> - **V1.5(市场调用方式)**:⑨ 市场服务改为**引用式安装**——用户"添加到我的服务"=建立**引用**(`mcp_services` 中 `source='marketplace'`,config 留空、不复制凭证),可加入分组,**经 `/mcp/group/:slug` 统一调用**;`source=marketplace` 服务按市场价扣费、`source=user` 自有服务免费。撤销 V1.4 的"禁止安装"与 `/mcp/marketplace/:slug` 直连端点。
> - V1.6(自用模式):⑩ 新增 `SelfUseModeEnabled`(参考 new-api `operation_setting.SelfUseModeEnabled`,默认 `false`):**自用模式**可用全局默认价;**非自用模式(默认)**市场上架/启用**必须显式定价**(`price_per_call>0` 或 `billing_type='free'`),否则拒绝。
> - V1.7(查漏补缺):⑪ 明确**计费口径**(仅 `tools/call` 与 Smart `mcp.execute` 扣费,握手/发现免费,§6.7);⑫ 市场项**下架/删除与引用生命周期 + 去重**(§11);⑬ 平台凭证**加密存储**(§4.3);⑭ 存量数据**迁移**(§4.3);⑮ 计费**幂等** `request_id` / FailOpen **欠账平账**(§6.3/§6.6);⑯ 调用日志 **payload 留存与脱敏**(§4.5)。
> - V1.8(虚拟服务约束):⑰ 视觉/摄像头等**虚拟服务**(`transport_type='virtual'`)改为**仅自有配置、自己免费使用**,`source` 恒为自身类型(vision/camera)、永不为 `marketplace`,**不可上架市场**——管理员手动添加 / 从自有服务克隆上架时对 `transport_type='virtual'` 一律拒绝(§5.6/§11/D16)。

---

## 1. 概述与目标

### 1.1 背景

NewMCP 当前已完成 V2 核心(MCP 网关、分组、市场、视觉/摄像头、调用日志、系统设置),整体完成度约 94%。
商业化在 [PRD.md](./PRD.md) V3 与 [ARCHITECTURE.md](./ARCHITECTURE.md) §7.3 已有规划(BillingService、plans/subscriptions/invoices 表、配额中间件、支付网关抽象),但未细化。

本文档定义商业化的**完整数据模型、计费链路、API、前端**设计,指导后续实现。设计参考 `reference/new-api` 的成熟计费体系,并按 MCP 场景做**结构性简化**。

### 1.2 核心差异:MCP 工具网关 vs LLM API 网关

| 维度 | new-api(LLM 网关) | new-mcp(MCP 工具网关) | 设计影响 |
|------|--------------------|------------------------|----------|
| 计费对象 | 一次 LLM 推理(prompt/completion tokens) | 一次 MCP 工具调用(tools/call) | **统一按次固定单价** |
| 成本确定性 | 不确定(实际 token 事后才知) | 确定(固定单价) | 可精确预扣,**无估算、无差额结算** |
| token 维度 | 核心维度(模型倍率×补全倍率×分组倍率) | 不计(vision 等内部 LLM 成本不重复计量) | **舍弃所有 token 维度** |
| 缓存/音频/图像 token | 精细化倍率 | 不适用 | **全部舍弃** |
| 阶梯表达式引擎 | `pkg/billingexpr` | 按次无需 | **舍弃** |

> **一句话结论**:商业化只对**市场来源服务(`source='marketplace'`)**按"**每个工具调用收一个固定价**"计费;**用户自有服务(`source='user'`)免费**。两者共用同一条分组调用路径,仅按来源决定是否计费。new-api 所有 token 倍率体系均不移植。

### 1.3 商业化模块全景

```
┌─────────────────────────────────────────────────────────────┐
│                    商业化模块 (Commercialization)            │
│                                                             │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐     │
│  │ MCP 价格设置 │   │ 用量计量     │   │ 用户额度管理 │     │
│  │ (Pricing)    │──>│ (Metering)   │──>│ (Quota)      │     │
│  │ 市场3级定价  │   │ 仅市场来源   │   │ 余额/调额/提醒│   │
│  └──────────────┘   └──────────────┘   └──────────────┘     │
│         ▲                  ▲                  ▲             │
│         │                  │                  │             │
│  ┌──────┴──────┐   ┌───────┴──────┐   ┌──────┴──────┐      │
│  │ 服务市场管理│   │ 计费链路     │   │ 充值/兑换   │      │
│  │(Marketplace)│   │(BillingSvc)  │   │(Topup/Redeem)│     │
│  │ 平台托管+定价│  │ 分组路径按   │  │ 额度入账    │      │
│  │ 引用式安装  │   │ source 计费  │  │             │      │
│  └─────────────┘   └──────────────┘   └─────────────┘      │
│                                                             │
│  统一调用路径 /mcp/group/:slug:                             │
│    source=user(自有) → 免费;source=marketplace(市场) → 扣费│
│                                                             │
│  ┌──────────────┐   ┌──────────────┐                        │
│  │ 订阅套餐(V2) │   │ 用量看板(V2) │                        │
│  └──────────────┘   └──────────────┘                        │
└─────────────────────────────────────────────────────────────┘
```

### 1.4 分期路线

| 阶段 | 模块 | 目标 |
|------|------|------|
| **V1(本文档重点)** | 市场服务 3 级定价 · 引用式安装 · 用量计量(仅市场来源) · 用户额度管理 · 计费链路(分组路径按 source) · 批量定价 · 兑换码 · 管理员调额 · 自有服务免费+开关 · 自用模式(强制定价) | 闭环"市场定价→引用添加→分组调用扣费→额度→兑换" |
| **V2** | 充值/在线支付 · 订阅套餐 · 用量看板/排行 · 工具级精确定价 UI · 市场工具自动同步 | 资金入口、套餐化、运营数据 |
| **V3** | 多租户/分账 · 成本核算(上游成本) · 推广返利 | 规模化运营 |

> 本文覆盖 V1 全部 + V2 设计(标注 V2),V3 仅占位。

---

## 2. 核心设计决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D1 | 计费模式 | **纯按次固定单价** | MCP 调用成本确定;vision 等内部 LLM 工具不重复计 token |
| D2 | 额度单位 | **统一整数 quota**,`QuotaPerUnit` 换算,复用 `User.Quota/UsedQuota` | 避免浮点精度;字段已存在([model/user.go:18-19](../model/user.go)) |
| D3 | 服务市场 | **平台托管 + 引用式安装**:管理员配置上游(平台持有连接/凭证);用户"添加到我的服务"=建立**引用**(不复制配置/凭证),可加入分组,经 `/mcp/group/:slug` 统一调用 | UX 统一(同一分组混用免费自有+付费市场);凭证不泄露;复用现有分组链路 |
| D4 | 定价粒度 | **市场服务**:工具级 > 服务级 > 全局默认(3 级);**自有服务**:免费 | 市场服务统一 3 级解析;自有服务不收费 |
| D5 | 计费时序 | **预扣(精确)→ 执行 → 成功确认 / 失败退款** | 成本确定,精确预扣,无估算、无差额结算 |
| D6 | 高频优化 | **信任额度旁路**(余额 > `TrustQuota` 跳过预扣,成功后补扣) | 高频用户减少 DB 写入 |
| D7 | 货币 | **单位无关整数 + 可配置展示货币**(默认 CNY) | 1 元 = `QuotaPerUnit` quota |
| D8 | 余额不足 | **拒绝本次调用 + 低额度邮件提醒,不禁用 Key** | 充值后立即可用,无需恢复 |
| D9 | 失败计费 | **上游错误全额退款**;客户端参数错误默认不计 | 体验优先 |
| D10 | 跨库 | **SQLite/MySQL/PostgreSQL 三库兼容**,保留字列用 GORM map 条件 | 遵循项目约定([memory: db-reserved-words]) |
| D11 | 兑换码列名 | 用 `code` 而非 `key` | `key` 是 SQL 保留字 |
| D12 | 自有服务 | `source='user'` 服务**免费,不扣费**;`UserOwnedServicesEnabled` 开关控制是否允许用户添加/调用**自有**服务(不影响市场引用) | 自有不纳入计费;开关支持"纯市场模式" |
| D13 | 管理员调额 | 在**用户管理**内增/减/设额度(参考 new-api `POST /api/user/manage` `add_quota`) | 运营手动调控用户余额 |
| D14 | 市场上架方式 | **克隆**(从自有服务深拷贝,无关联,凭证保留并提示替换)/ **手动添加**,均生成自包含 `marketplace_items` | 多管理员共享管理;市场用平台凭证承担上游成本 |
| D15 | 自用模式 | `SelfUseModeEnabled`:**自用模式**可用全局默认价;**非自用(默认)**市场上架/启用必须显式定价 | 参考 new-api `operation_setting.SelfUseModeEnabled`;防止商业部署下服务误用默认价/免费上架 |
| D16 | 虚拟服务 | 视觉/摄像头等**虚拟服务**(`transport_type='virtual'`,`source`∈{vision,camera})**仅自有配置、自己免费使用**,`source` 恒不为 `marketplace`,**不可上架市场**(手动添加/从自有服务克隆均拒绝 `transport_type='virtual'`) | 虚拟服务的 config/凭证绑定配置者私有资源(如 `vision_configs.ref_id`),无平台可托管的上游连接;属内置 handler,不应进入计费/市场流通 |

---

## 3. 额度单位与货币

### 3.1 统一整数 quota

所有金额最终折算为**整数 quota**(配额点),内部结算只做整数运算。

```
QuotaPerUnit = 500000   // 1 展示货币单位 = 500000 quota(可配置)
                       // 即 1 quota = 0.000002 元(默认 CNY)
```

- 管理员在前端按"**元/次**"配置价格,后端落库为 `price_per_call DECIMAL`,加载到内存时换算为整数 `pricePerCallQuota = round(price_per_call * QuotaPerUnit)`,计费只动整数。
- 充值/兑换:入账 quota = `金额 × QuotaPerUnit`。
- 展示:前端 quota ÷ QuotaPerUnit = 元。

> 与 new-api `common/constants.go:62` 的 `QuotaPerUnit = 500*1000.0` 语义一致。

### 3.2 字段复用(已存在,仅激活)

| 模型 | 字段 | 现状 | 本期动作 |
|------|------|------|----------|
| `User` | `Quota int64` ([model/user.go:18](../model/user.go)) | 已存在,默认 0,只写不扣 | 激活扣减逻辑 |
| `User` | `UsedQuota int64` ([model/user.go:19](../model/user.go)) | 已存在 | 累加已消耗 |
| `User` | `RequestCount int64` ([model/user.go:20](../model/user.go)) | 已在 recordLog 自增 | 保持 |
| `User` | `Group string` ([model/user.go:21](../model/user.go)) | 用户套餐分组(default/vip/svip) | **分组倍率作用对象** |
| `ApiKey` | `Quota/UsedQuota/UnlimitedQuota` ([model/api_key.go:21-23](../model/api_key.go)) | 已存在 | 激活为**单 Key 消费上限** |

> **关键澄清**:`User.Group`(用户套餐分组,用于限流与分组倍率)与 `mcp_groups`(用户的服务聚合容器)是**两套独立概念**。分组倍率(§5.3)作用于 `User.Group`。

### 3.3 现有函数现状

- `IncreaseUserQuota(id, quota)` ([model/user.go:98](../model/user.go)) — 可用,充值/兑换/退款/管理员调增调用。
- `DecreaseUserQuota(id, quota)` ([model/user.go:106](../model/user.go)) — **已定义但全代码库从未调用**(额度是"死字段")。本期计费链路将首次调用它。

> ⚠️ 现有 `DecreaseUserQuota` 需复核:是否带 `quota >= ?` 防透支原子条件。若否,需改为 `UPDATE ... SET quota = quota - ? WHERE id = ? AND quota >= ?`(GORM `gorm.Expr` + map 条件)。参考 new-api `model.DecreaseUserQuota` / `DeltaUpdateUserQuota`([model/user.go:964](../reference/new-api/model/user.go))。

---

## 4. 数据模型扩展

> 建表 SQL 为 MySQL 语法(沿用 [DATABASE.md](./DATABASE.md) 风格)。GORM `AutoMigrate` 注册点:[model/main.go:67-84](../model/main.go) `migrateDB`。新增表/列须三库兼容(见 §17 风险)。

### 4.1 `users` 表 — 新增计费相关列

现有 `users` 表的 `quota`/`used_quota`/`request_count`/`group`/`remark` 等列在 [DATABASE.md](./DATABASE.md) §2.2 中**已过时未同步**,需先补齐文档。本期新增:

```sql
ALTER TABLE `users`
    ADD COLUMN `billing_preference` VARCHAR(16) DEFAULT 'wallet_only'
        COMMENT '计费来源偏好: wallet_only(V1唯一) / wallet_first / subscription_first(V2)',
    ADD COLUMN `total_topup` BIGINT DEFAULT 0
        COMMENT '累计充值额度(quota),审计用';
```

> V1 只有钱包(`wallet_only`),`billing_preference` 字段先建好,为 V2 订阅预留。

### 4.2 `mcp_services` 表 — 复用 source/marketplace_item_id(引用式安装)

用户"添加市场服务到我的服务"时,创建一条 `mcp_services` **引用行**(相关字段已存在,见 [DATABASE.md](./DATABASE.md) §2.4 `source`/`marketplace_item_id`):

```sql
-- 字段已存在,仅说明引用式安装的取值约定(无需 ALTER):
--   source              = 'marketplace'      (标识为市场引用)
--   marketplace_item_id = <item.id>
--   transport_type      = 'marketplace'      (哨兵值,resolver 见此改用平台 session)
--   config              = '{}'               (空:不复制上游配置/凭证)
--   tools_cache         = 复制自 marketplace_items.tools_snapshot(供分组聚合)
--   display_name/description/icon_url = 复制自市场项
```

> **引用式安装(D3)**:`config` 留空,**上游配置/凭证仍由平台在 `marketplace_items` 侧托管**,用户拿不到平台凭证。用户不可编辑市场引用服务的上游配置(平台托管);可像普通服务一样加入分组、经 `/mcp/group/:slug` 调用。
>
> **计费区分**:`source='user'`(自有)免费;`source='marketplace'`(市场引用)按市场价计费。二者可共存于同一分组。
>
> **工具同步(V1 限制)**:添加时复制 `tools_cache` 快照;平台侧工具变更需用户"重新同步"(V2 提供自动同步)。

### 4.3 `marketplace_items` 表 — 平台托管服务 + 服务级定价

```sql
ALTER TABLE `marketplace_items`
    ADD COLUMN `billing_type` VARCHAR(16) NOT NULL DEFAULT 'per_call'
        COMMENT '服务级计费类型: free(免费标记), per_call',
    ADD COLUMN `price_per_call` DECIMAL(10,4) NOT NULL DEFAULT 0.0000
        COMMENT '服务级按次单价(展示货币);free 时忽略',
    ADD COLUMN `subscription_only` TINYINT DEFAULT 0
        COMMENT '0=按次计费, 1=仅订阅用户可用(V2);V1固定0';
```

> **服务市场商业化(D3)——平台托管 + 引用式安装**:
> - 管理员上架时配置上游连接(`transport_type` + `config_template`,**平台统一持有连接、维护 session 与工具缓存,凭证不暴露给用户**)。
> - **上架方式(D14)**:**手动添加** 或 **从自有服务克隆**(深拷贝 transport/config/auth,与源服务无关联;克隆时保留源凭证但前端高亮提示替换为平台凭证,避免复用个人 Key 承担全平台流量)。
> - **用户侧**:浏览市场 → **"添加到我的服务"**(`POST /marketplace/:id/add`)→ 生成 `mcp_services` 引用行(§4.2)→ 可加入分组 → 经 `/mcp/group/:slug` 调用 → 按市场价扣费。
> - 定价走**工具级 > 服务级 > 全局默认** 3 级解析(§5.2);服务级即本表 `billing_type`/`price_per_call`。**批量定价**:`PUT /admin/marketplace/pricing/batch`。
> - **源码型(`category='source'`)在托管模型下不再适用**,降级为文档展示或下线;市场仅保留平台托管型(原 `instant`)。
> - **平台凭证加密(安全)**:`config_template` 含平台上游凭证,**敏感字段加密落库**(参考 `vision_configs.api_key` 加密存储);API 返回掩码,仅平台侧调上游时解密,管理员/用户均无法读取明文凭证。
> - **存量迁移**:开启计费后,存量市场项默认 `billing_type='per_call'`+`price=0` → 非自用模式判为"未定价"无法启用;需经批量定价(§5.5)显式设价后方可启用。

### 4.4 `mcp_tool_prices` 表 — 工具级定价(市场服务,V1可选/V2 UI)

```sql
CREATE TABLE `mcp_tool_prices` (
    `id`                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `marketplace_item_id` BIGINT UNSIGNED NOT NULL COMMENT '市场服务 ID(marketplace_items.id)',
    `tool_name`           VARCHAR(255)    NOT NULL COMMENT '原始工具名(不含命名空间前缀)',
    `billing_type`        VARCHAR(16)     NOT NULL DEFAULT 'per_call' COMMENT 'free, per_call',
    `price_per_call`      DECIMAL(10,4)   DEFAULT 0.0000,
    `enabled`             TINYINT         DEFAULT 1,
    `created_at`          DATETIME        DEFAULT CURRENT_TIMESTAMP,
    `updated_at`          DATETIME        DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_item_tool` (`marketplace_item_id`, `tool_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='市场服务工具级定价覆盖表';
```

> 命中即生效,优先级最高(定价解析第 1 级)。仅 `free`/`per_call`。V1 可先靠服务级 + 全局默认;V2 提供按工具调价 UI。

### 4.5 `mcp_call_logs` 表 — 扩展计费列(用量计量明细)

现有 `mcp_call_logs` ([model/mcp_call_log.go:9-31](../model/mcp_call_log.go)) 是纯审计日志。本期**在同一行追加计费列**,避免双写:

```sql
ALTER TABLE `mcp_call_logs`
    ADD COLUMN `billing_status` VARCHAR(16) DEFAULT 'skipped'
        COMMENT '计费状态: skipped(自有服务/未启用/免费), charged(已扣), refunded(失败退款), blocked(余额不足拒绝)',
    ADD COLUMN `billing_type`   VARCHAR(16) DEFAULT NULL  COMMENT '本次解析到的计费类型',
    ADD COLUMN `unit_price`     DECIMAL(10,6) DEFAULT NULL COMMENT '本次单价(展示货币,快照)',
    ADD COLUMN `quota_consumed` BIGINT DEFAULT 0  COMMENT '本次实扣额度(quota);自有服务/退款/拒绝为0',
    ADD COLUMN `price_scope`    VARCHAR(16) DEFAULT NULL COMMENT '定价来源层级: tool/service/marketplace/global;自有服务为 NULL',
    ADD COLUMN `marketplace_item_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '市场来源服务调用时关联的市场项 ID';
```

> 计费与审计合一:每次 `tools/call` 写一条 log,计费结果同落。**自有服务**(`source=user`)写 `billing_status='skipped'`、`quota_consumed=0`、`price_scope=NULL`;**市场引用服务**(`source=marketplace`)写实际计费结果 + `marketplace_item_id`。`quota_consumed` 是用户账单的权威明细。
> ⚠️ 现有 `recordLog` ([internal/mcp/handler/gateway_handler.go:599](../internal/mcp/handler/gateway_handler.go)) 未写 `ResponsePayload`;本期顺带补全,便于对账。
>
> **留存与隐私**:`request_payload`/`response_payload` 可能含敏感数据且量级大,需配置 **TTL 清理**(`LogRetentionDays`,默认 30 天)与**敏感字段脱敏**;由 `LogPayloadEnabled`(默认 true)控制是否落 payload,关闭时只存元数据 + 计费列(见 §15)。

### 4.6 `mcp_usage_hourly` 表 — 小时聚合(用量看板,V2)

```sql
CREATE TABLE `mcp_usage_hourly` (
    `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id`       BIGINT UNSIGNED NOT NULL,
    `marketplace_item_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '市场来源服务(计费对象)',
    `tool_name`     VARCHAR(255)    DEFAULT '',
    `bucket_hour`   BIGINT          NOT NULL COMMENT 'Unix 小时时间戳(对齐)',
    `call_count`    INT             DEFAULT 0,
    `success_count` INT             DEFAULT 0,
    `quota_sum`     BIGINT          DEFAULT 0 COMMENT '消耗 quota 之和',
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_user_hour_item_tool` (`user_id`, `bucket_hour`, `marketplace_item_id`, `tool_name`),
    KEY `idx_bucket_hour` (`bucket_hour`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用量小时聚合表(V2 看板,仅市场来源服务)';
```

> 参考 new-api `QuotaData` ([model/usedata.go:13-26](../model/usedata.go)):内存批量缓存 + 定时刷库。V1 仅写明细,聚合 V2 实现。

### 4.7 `api_keys` 表 — 无新增(沿用现有 `status`)

> 余额不足**不禁用 Key**(参考 new-api)。现有 `api_keys.status`(1=启用/2=禁用)仅用于管理员/用户**手动**启停。

### 4.8 `redemptions` 表 — 兑换码(V1)

```sql
CREATE TABLE `redemptions` (
    `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `code`          CHAR(32)        NOT NULL COMMENT '兑换码(UUID 去连字符,32位)',
    `name`          VARCHAR(128)    DEFAULT '' COMMENT '兑换码名称/备注',
    `quota`         BIGINT          NOT NULL COMMENT '面值(quota)',
    `status`        TINYINT         DEFAULT 1 COMMENT '1=可用, 2=已兑换, 3=已禁用',
    `user_id`       BIGINT UNSIGNED DEFAULT NULL COMMENT '兑换者用户 ID',
    `expired_at`    BIGINT          DEFAULT 0 COMMENT '过期时间戳,0=永不过期',
    `created_at`    DATETIME        DEFAULT CURRENT_TIMESTAMP,
    `redeemed_at`   DATETIME        DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_code` (`code`),
    KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='兑换码表';
```

> 参考 new-api `Redemption` ([model/redemption.go:14-27](../model/redemption.go))。列名用 `code`(D11)。兑换用事务 + `FOR UPDATE` 行锁 + 幂等校验(参考 `Redeem()` [model/redemption.go:115-156](../model/redemption.go))。

### 4.9 `topups` 表 — 充值订单(V2)

```sql
CREATE TABLE `topups` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id`         BIGINT UNSIGNED NOT NULL,
    `amount`          BIGINT          NOT NULL COMMENT '入账额度(quota)',
    `money`           DECIMAL(10,2)   NOT NULL COMMENT '支付金额(展示货币)',
    `trade_no`        VARCHAR(64)     NOT NULL COMMENT '唯一订单号',
    `payment_method`  VARCHAR(32)     DEFAULT '' COMMENT 'epay / alipay / stripe / balance',
    `status`          VARCHAR(16)     DEFAULT 'pending' COMMENT 'pending / success / expired',
    `created_at`      DATETIME        DEFAULT CURRENT_TIMESTAMP,
    `paid_at`         DATETIME        DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_trade_no` (`trade_no`),
    KEY `idx_user_status` (`user_id`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='充值订单表(V2)';
```

> 参考 new-api `TopUp` ([model/topup.go:14-25](../model/topup.go))。入账:`quota = money * QuotaPerUnit`。V1 暂用兑换码 + 管理员调额替代,V2 接支付网关。

### 4.10 订阅套餐(V2)

```sql
CREATE TABLE `subscription_plans` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `title`           VARCHAR(128)    NOT NULL,
    `price_amount`    DECIMAL(10,2)   NOT NULL COMMENT '展示价格',
    `duration_value`  INT             NOT NULL COMMENT '时长数值',
    `duration_unit`   VARCHAR(16)     NOT NULL COMMENT 'day/month/year',
    `quota_total`     BIGINT          NOT NULL COMMENT '套餐内含额度(quota);0=无限',
    `upgrade_group`   VARCHAR(32)     DEFAULT '' COMMENT '购买后升级到的用户分组',
    `max_purchase`    INT             DEFAULT 0 COMMENT '每用户最大购买数,0=不限',
    `status`          TINYINT         DEFAULT 1,
    `created_at`      DATETIME        DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='订阅套餐(V2)';

CREATE TABLE `user_subscriptions` (
    `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id`       BIGINT UNSIGNED NOT NULL,
    `plan_id`       BIGINT UNSIGNED NOT NULL,
    `amount_total`  BIGINT          NOT NULL COMMENT '套餐总额度',
    `amount_used`   BIGINT          DEFAULT 0,
    `start_time`    BIGINT          NOT NULL,
    `end_time`      BIGINT          NOT NULL COMMENT '到期时间戳',
    `status`        VARCHAR(16)     DEFAULT 'active' COMMENT 'active/expired/cancelled',
    PRIMARY KEY (`id`),
    KEY `idx_user_status` (`user_id`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户订阅实例(V2)';
```

> 参考 new-api `SubscriptionPlan`/`UserSubscription` ([model/subscription.go:146-281](../model/subscription.go))。V2 **简化**:去掉周期性配额重置、交叉倍率等。

### 4.11 ER 关系(商业化部分)

```
users (1) ──< (N) redemptions         (兑换记录)
users (1) ──< (N) topups              (充值订单, V2)
users (1) ──< (N) user_subscriptions  (订阅, V2)
users (1) ──< (N) mcp_call_logs       (计费明细, quota_consumed)
users (1) ──< (N) mcp_usage_hourly    (聚合, V2)
users (1) ──< (N) api_keys            (Key 级消费上限,手动启停)
users (1) ──< (N) mcp_services        (含 source=user 自有 + source=marketplace 市场引用)

marketplace_items (1) ──< (N) mcp_tool_prices   (工具级定价)
marketplace_items (1) ──< (N) mcp_services      (用户添加的市场引用, source=marketplace)
marketplace_items (1) ──< (N) mcp_call_logs     (市场来源调用明细, marketplace_item_id)

mcp_groups (1) ──< (N) mcp_group_services >── mcp_services
  (一个分组可同时含 source=user 免费服务 与 source=marketplace 付费市场引用)

定价解析(运行时,仅 source=marketplace 服务):
  mcp_tool_prices(marketplace_item_id, tool_name)
    > marketplace_items.billing_type / price_per_call
      > options.BillingDefaultPricePerCall
```

### 4.12 索引策略

| 场景 | 索引 | 说明 |
|------|------|------|
| 计费明细查询(用户账单) | `mcp_call_logs(user_id, created_at)` | 已存在,按时间范围分页 |
| 按市场服务/工具统计 | `mcp_call_logs(marketplace_item_id)`/`(tool_name)` | 新增列 |
| 用户的引用服务查询 | `mcp_services(user_id, source, marketplace_item_id)` | 找出用户添加的市场引用 |
| Key 手动启停查询 | `api_keys(user_id, status)` | 管理员/用户启停 |
| 兑换码核销 | `redemptions(code)` UNIQUE | O(1) 查找 |
| 充值订单幂等 | `topups(trade_no)` UNIQUE | 防重复入账 |
| 看板聚合 | `mcp_usage_hourly(user_id, bucket_hour)` | 时间序列 |

---

## 5. 定价体系(MCP 价格设置)

### 5.1 计费类型 `billing_type`

| 类型 | 语义 | 适用 | 计费公式(quota) |
|------|------|------|------------------|
| `free` | 免费 | 公益/体验市场服务 | 0 |
| `per_call` | 按次固定单价(默认) | **市场来源服务所有工具(含 vision 等内部 LLM 工具)** | `pricePerCallQuota × groupRatio` |

> 仅两种类型。**用户自有服务(`source=user`)一律免费**(不走定价)。不存在 per_token。

### 5.2 定价解析(市场来源服务,3 级,D4)

**仅 `source='marketplace'` 服务计费**(在分组调用路径上按 source 触发,§6);`source='user'` 自有服务免费。

```
resolveMarketplacePrice(item, toolName, userGroup):
  1. 工具级: mcp_tool_prices[item.id, toolName]            → 命中即用(最高优先)
  2. 服务级: marketplace_items.billing_type/price_per_call  → 非 NULL 即用
  3. 全局默认: options.BillingDefaultPricePerCall + BillingDefaultType(**仅自用模式生效**,§5.6)
  4. 兜底:   BillingEnabled=false 或解析失败 → free(不计费)

最终 quota = baseQuota × groupRatio(user.Group)
```

> 设计意图:管理员设全局默认价覆盖全站市场服务,再对个别市场服务(服务级)或其中某个工具(工具级)精确调整。
>
> **自用模式门控(D15)**:全局默认(第 3 级)仅在 `SelfUseModeEnabled=true` 时作为兜底;非自用模式下市场上架/启用已强制显式定价(§5.6),解析必命中第 1-2 级,未定价则调用时报错(参考 new-api `relay/helper/price.go:23`)。

### 5.3 分组倍率 `group_ratio`

作用于 `User.Group`(用户套餐分组):

```
finalQuota = baseQuota × groupRatio(user.Group)

groupRatio 配置(Option, JSON):
  {"default": 1.0, "vip": 0.8, "svip": 0.6}
```

> 参考 new-api `group_ratio` ([setting/ratio_setting/group_ratio.go:12-16](../setting/ratio_setting/group_ratio.go))。**V1 只做单层全局分组倍率**,舍弃交叉倍率。分组选项复用现有 `UserGroupOptions`([model/option.go:111](../model/option.go))。

### 5.4 价格内存缓存

参考 new-api `GetPricing()` 1 分钟 TTL + double-check locking([model/pricing.go:66-78](../model/pricing.go)):
- 服务级价格随市场项查询附带读取,可加内存 LRU。
- 全局默认/分组倍率走现有 `OptionMap`([model/option.go:17](../model/option.go))内存缓存。
- 管理员改价后调用 `InvalidatePricingCache()` 强制刷新(参考 [model/pricing_refresh.go](../model/pricing_refresh.go))。

### 5.5 价格设置入口(管理员)

| 位置 | 配置对象 | API | 说明 |
|------|----------|-----|------|
| 全局默认价 / 分组倍率 / 计费开关 | options | `PUT /api/v1/admin/settings` | 逐 key 更新 |
| 市场项服务级定价(单个) | `marketplace_items` | `PUT /api/v1/admin/marketplace/:id` | 扩展现有上架编辑 |
| **市场项服务级定价(批量)** | `marketplace_items` | `PUT /api/v1/admin/marketplace/pricing/batch` | **多选已上架服务批量设价** |
| 工具级定价 | `mcp_tool_prices` | `PUT /api/v1/admin/marketplace/:id/tools/:tool/pricing`(V2) | 按工具精确调价 |

**批量定价请求体示例**:
```json
{
  "items": [
    { "id": 1, "billing_type": "per_call", "price_per_call": 0.05 },
    { "id": 2, "billing_type": "free" },
    { "id": 3, "billing_type": "per_call", "price_per_call": 0.10 }
  ]
}
```

### 5.6 自用模式与强制定价(D15,参考 new-api)

参考 new-api `operation_setting.SelfUseModeEnabled`([setting/operation_setting/operation_setting.go:6](../reference/new-api/setting/operation_setting/operation_setting.go),默认 `false`)、`controller/model.go:209`(`acceptUnsetRatioModel := SelfUseModeEnabled`)与 `relay/helper/price.go:23`("价格未配置"校验)。

| `SelfUseModeEnabled` | 全局默认价 | 市场上架/启用 | 调用时未定价 |
|---|---|---|---|
| `true`(自用模式) | **可用**(第 3 级兜底) | 允许无显式价格上架(继承全局默认) | 解析到全局默认 |
| `false`(非自用/商业,**默认**) | **不生效**(配置项前端隐藏) | **必须显式定价**:`price_per_call>0` 或 `billing_type='free'`,否则拒绝上架/启用 | 报错"价格未配置" |

- **"已显式定价"判定**:`billing_type='free'`(显式免费) 或 (`billing_type='per_call'` 且 `price_per_call>0`)。默认行(`per_call` + `price=0`)视为**未定价**。
- **上架/启用门控**(非自用模式):`POST /admin/marketplace`、`POST /admin/marketplace/clone`、`PUT /admin/marketplace/:id`(启用/上架)校验显式定价,不满足返回 400。
- **虚拟服务上架禁令(D16,硬约束)**:**与定价/自用模式无关**——`transport_type='virtual'` 的服务(vision/camera 等)**永远不可上架市场**:`POST /admin/marketplace`(手动添加,拒 `transport_type='virtual'`)与 `POST /admin/marketplace/clone`(从自有服务克隆,源服务 `transport_type='virtual'` 拒绝)直接返回 400。虚拟服务仅供配置者自己免费调用,详见 §11。
- **可设于初始化**:参考 new-api 在 setup 阶段询问(`controller/setup.go:23`),可选;主入口为系统设置。
- **关联行为(可选,参考 new-api classic)**:自用模式下可隐藏公开注册(已有独立 `RegisterEnabled` 开关,二者可联动)。

---

## 6. 计费链路(用量计量核心)

### 6.1 统一调用路径 + source 门控(D3 核心变化)

市场服务与自有服务**共用同一条分组调用路径**,仅按 `service.source` 决定是否计费——不再为市场另建端点。

```
POST /mcp、/smart/mcp、/mcp/group/:slug  (tools/call)
  → APIKeyAuth() + RateLimit()
  → internal/mcp/handler/gateway_handler.go:154  handleToolsCall()
  → routeAndCall() 解析目标服务:
       source='user'/'admin'   → 读 service.config,用户 session      → 【免费,不扣费】
       source='marketplace'    → 读 marketplace_items.config_template,平台 session → 【按 3 级定价计费】
  → 【插入点 A:预扣】 仅 source='marketplace' 时
  → :549 routeOrConnect() 转发上游(市场服务用平台 session,按 marketplace_item_id 复用)
  → 【插入点 B:确认/退款】 仅 source='marketplace' 时;recordLog 写计费列
```

**resolver 改造点**(`routeOrConnect`):
- `source=user/admin`:沿用现状(读 `service.config`,session 按 `service.id`)。
- `source=marketplace`:不读用户 config(为空),改读 `marketplace_items[service.marketplace_item_id].config_template + transport_type`,session **按 `marketplace_item_id` 复用平台连接**(同一市场项所有用户共享平台 session,使用平台凭证)。

**两个计费插入点**(仅 `source=marketplace` 触发):
- **A. 预扣**(`gateway_handler.go:223` 前):解析 3 级价格 → 原子扣额度。余额不足 → 拒绝本次调用 + 返回错误(不禁用 Key),不调上游。
- **B. 确认/退款**(`recordLog` 内,`gateway_handler.go:236/599`):成功确认(已扣)、失败退款(全额),同写计费列。

> 此架构比"市场专用端点"更优:市场服务天然融入用户的分组/API Key 工具视图,可在同一分组混用免费自有 + 付费市场服务;计费仅在 resolver 命中 `source=marketplace` 时介入,自有服务路径零改动。

### 6.2 两段式计费(预扣 → 执行 → 确认/退款)

成本确定(固定单价),故**无估算、无差额结算**:预扣即应扣全额。

```
┌────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│MCP     │    │Gateway   │    │Billing   │    │上游 MCP  │    │DB        │
│Client  │    │Handler   │    │Service   │    │Server    │    │          │
└───┬────┘    └────┬─────┘    └────┬─────┘    └────┬─────┘    └────┬─────┘
    │ tools/call   │               │               │               │
    │ (分组端点)   │ routeAndCall 解析 service      │               │
    │─────────────>│───┐           │               │               │
    │              │<──┘ source?   │               │               │
    │              │               │               │               │
    │              │ source=user   → 直接转发(免费,billing_status=skipped)
    │              │               │               │               │
    │              │ source=marketplace:           │               │
    │              │ resolvePrice(3级) ───────────>│ ① 预扣 quota  │
    │              │               │───────────────────────────────>│ UPDATE quota=quota-? WHERE quota>=?
    │              │               │  (余额不足 → 返回 ErrInsufficient,不禁用 Key)
    │              │<──────────────│ (BillingSession)               │
    │              │ ② 转发(平台 session, 按 item_id)              │
    │              │───────────────────────────────>│ 执行工具      │
    │              │<───────────────────────────────│               │
    │              │ ③a 成功: Confirm(no-op,已扣) / ③b 失败: Refund(全额退)
    │              │──────────────>│ 写 mcp_call_logs(marketplace_item_id, quota_consumed, billing_status)
    │              │               │  (成功且余额低于阈值 → 异步发低额度邮件)
    │              │               │───────────────────────────────>│
    │ 结果         │               │               │               │
    │<─────────────│               │               │               │
```

### 6.3 `BillingService` 接口设计

```go
// service/billing.go (新建,仅 source=marketplace 时由分组调用路径触发)

// BillingSession 一次市场服务调用的计费会话(参考 new-api billing_session.go:25-36, 大幅简化)
type BillingSession struct {
    UserID          int64
    ApiKeyID        int64
    MarketplaceItemID int64
    ConsumedQuota   int64  // 已预扣 = 应扣(成本确定,二者相等)
    Trusted         bool   // 命中信任旁路,未实际预扣
}

// PreConsume 预扣:解析 3 级价格 → 原子扣用户额度 + Key 额度。
//   余额不足 → 返回 ErrInsufficientQuota(参考 new-api PreConsumeQuota:HTTP 403,
//   ErrOptionWithSkipRetry + NoRecordErrorLog,视为业务拒绝而非系统错误),**不禁用 Key**。
//   命中信任旁路(余额 > TrustQuota):不扣,Trusted=true,成功后由 Confirm 补扣。
func (s *BillingService) PreConsume(ctx context.Context, p PriceInfo) (*BillingSession, error)

// Confirm 成功确认:Trusted 时补扣 ConsumedQuota;否则 no-op(预扣已完成)。
//   累加 User.UsedQuota 与 ApiKey.UsedQuota,写 mcp_call_logs(billing_status=charged)。
//   若扣后余额 < QuotaRemindThreshold,异步发低额度邮件。
func (s *BillingService) Confirm(sess *BillingSession) error

// Refund 失败退款:Trusted 时无需退(未扣);否则全额 IncreaseUserQuota(幂等)。
//   写 mcp_call_logs(billing_status=refunded, quota_consumed=0)。
func (s *BillingService) Refund(sess *BillingSession) error
```

> 扣费原子性:用户额度 `UPDATE users SET quota = quota - ? WHERE id = ? AND quota >= ?`(GORM `gorm.Expr`,保留字列用 map 条件);Key 额度同理。受影响行数=0 即余额不足 → 返回 `ErrInsufficientQuota`(参考 new-api `ErrorCodeInsufficientUserQuota`),不禁用 Key。
>
> **幂等(防重扣)**:预扣/退款以 MCP **request_id** 为幂等键(参考 new-api 订阅预扣 `SubscriptionPreConsumeRecord` 的 `request_id` 唯一索引)——网关"预扣后、上游调用前"崩溃等异常重试场景不重复扣费;`Refund` 天然幂等(已退不再退)。

### 6.4 信任额度旁路(高频优化)

参考 new-api `GetTrustQuota`([service/pre_consume_quota.go:45-64](../reference/new-api/service/pre_consume_quota.go)):

```
若 user.Quota > TrustQuota (默认 10 × QuotaPerUnit = 10 元):
    PreConsume 不实际扣 DB,只 Trusted=true
    调用成功 → Confirm 一次性补扣 ConsumedQuota
    调用失败 → 无需退款(未扣)
否则:
    正常预扣 + 失败退款 + 余额不足拒绝
```

### 6.5 余额不足处理(new-api 风格,D8)

参考 new-api `PreConsumeQuota`([service/pre_consume_quota.go:33-79](../reference/new-api/service/pre_consume_quota.go)):

```
PreConsume 发现 user.Quota <= 0 或 user.Quota - 预扣 < 0(或 apiKey 预算不足):
  1. 返回 ErrInsufficientQuota(HTTP 403,ErrOptionWithSkipRetry + NoRecordErrorLog,
     视为业务拒绝而非系统错误)
  2. 计费链路写 mcp_call_logs(billing_status='blocked', quota_consumed=0),不调用上游
  3. 返回 MCP JSON-RPC error(QUOTA_INSUFFICIENT)
  不禁用 Key,不触发任何禁用状态变更
```

**低额度提醒**(参考 new-api `checkAndSendQuotaNotify` [service/quota.go:452](../reference/new-api/service/quota.go)):
- 在**成功消费后**(Confirm)检查:`user.Quota - 本次消费 < QuotaRemindThreshold` → 异步发"额度即将用尽"邮件(复用现有 SMTP)。
- threshold = 全局 `QuotaRemindThreshold`(Option),默认 0=不提醒。

> Key 全程保持 enabled,用户兑换/充值后**立即**可继续调用,无需任何恢复操作。

### 6.6 失败与边界处理

| 场景 | 处理 |
|------|------|
| 自有服务(`source=user`)调用 | 免费,`billing_status='skipped'`,`quota_consumed=0` |
| 市场服务(`source=marketplace`)余额不足 | 拒绝本次调用(§6.5),返回错误,**不调上游**;不禁用 Key |
| 上游 MCP 返回错误 | `Refund()` 全额退,`billing_status='refunded'`,`quota_consumed=0` |
| 客户端参数错误(上游 4xx) | 默认同上不收费;可配 `ChargeOnClientError` |
| 工具超时 | 视为失败退款(或可配 `ChargeOnTimeout`) |
| `BillingEnabled=false` | 市场服务也跳过计费,`billing_status='skipped'` |
| 管理员用户(`role=admin/super_admin`) | 默认免计费(`skipped`),可配 |
| 计费 DB 异常 | 默认 **FailOpen**:放行调用并记欠账(`billing_status='debt'` + 欠账计数),不阻断;可配 `BillingFailOpen=false` 拒绝。**欠账平账**:下次调用预扣时补扣 / 定时对账任务 `BillingDebtReconcile`(V2) |

### 6.7 计费口径(哪些 MCP 方法扣费)

| MCP 方法 | 计费 | 说明 |
|----------|------|------|
| `tools/call`(Direct) | ✅ 扣费 | 命中 `source=marketplace` 服务按市场价扣;`source=user` 免费 |
| `mcp.execute`(Smart) | ✅ 扣费 | 本质转发到上游工具,resolver 解析底层服务后同 `tools/call` 规则(命中 marketplace 才扣) |
| `initialize` | ❌ 免费 | 协议握手 |
| `tools/list`(Direct/Smart) | ❌ 免费 | 工具发现 |
| `mcp.search` / `mcp.describe`(Smart) | ❌ 免费 | 搜索/描述元工具(只读发现) |

> **判定原则**:只有**实际执行上游工具**(`tools/call`、`mcp.execute`)才扣费;握手与发现类一律免费。Smart 模式下 `mcp.execute` 必须解析到目标服务的 `source`,命中 marketplace 才计费。
> **虚拟工具**(vision/camera 等,`transport_type='virtual'`)属用户自有,**保持免费**;**永不可上架为市场服务**(D16/§11:手动添加/克隆上架均拒绝 `transport_type='virtual'`),仅自有配置、配置者自己使用。

---

## 7. 用户额度管理

### 7.1 额度生命周期

```
            ┌─────────── 注册赠送 (QuotaForNewUser, 可配) ──────────┐
            │                                                       ▼
  充值(V2) ──> [ User.Quota (可用余额) ] <── 兑换码 ── 退款(市场服务失败)
                     │   ▲                  <── 管理员调额(D13)
            消费扣减  │   │ 信任旁路事后补扣      (仅市场来源服务消费)
                     ▼   │
              [ User.UsedQuota (累计已用) ]  <── 每次市场服务成功调用累加

  余额不足 ──> 拒绝本次市场服务调用(Key 保持可用) ── 充值后立即可用
  余额 < QuotaRemindThreshold ──> 异步发"额度即将用尽"邮件
```

- **入账**:注册赠送、兑换码、充值(V2)、**管理员调额**、失败退款 → `IncreaseUserQuota` + 写流水。
- **出账**:**仅市场来源服务**成功调用扣减 → `DecreaseUserQuota`(原子) + `UsedQuota += consumed`。自有服务不扣。
- **流水**:消费在 `mcp_call_logs` 留痕;充值/兑换/调额 V2 可加 `balance_changes` 通用流水表对账。

### 7.2 管理员调额(D13,参考 new-api)

管理员在**用户管理**页对单个用户增/减/设额度,参考 new-api `POST /api/user/manage`(`action=add_quota`,[controller/user.go:1068](../reference/new-api/controller/user.go)):

- **new-mcp 接口**:`POST /api/v1/admin/users/:id/quota`,请求体 `{ "mode": "add|sub|set", "value": 500000, "remark": "补偿" }`(`value` 单位为 quota)。
- **mode 映射 new-api**:`add` → `IncreaseUserQuota`;`sub` → `DecreaseUserQuota`;`set` → 覆盖 `Quota`。
- **权限校验**:参考 new-api `canManageTargetRole`([controller/user.go:1018](../reference/new-api/controller/user.go))——管理员不得操作同级或更高角色用户的额度。
- **审计**:每次调额写审计日志(参考 new-api `recordManageAuditFor` "user.quota_add/sub/set")。
- **创建用户时设额**:`POST /admin/users` 支持初始 `quota` 字段(已有)。
- **前端**:用户管理页新增**额度调整对话框**(参考 new-api `features/users/components/user-quota-dialog.tsx` → `adjustUserQuota`),展示当前余额/已用,支持增/减/覆盖三种操作 + 备注。

### 7.3 ApiKey 级消费上限

`ApiKey.Quota/UnlimitedQuota`([model/api_key.go:21-23](../model/api_key.go))作为**单 Key 消费上限**(仅对市场来源服务消费生效),与用户总额度双约束(参考 new-api `PreConsumeTokenQuota`):
- **语义**:`quota` 是**可消耗额度上限**(随市场服务调用递减,**非周期重置**),可在 API Key 管理页**编辑增减**或设 `unlimited_quota` 无限;用尽即拒,不禁用。
- 市场服务扣费同步 `apiKey.used_quota += consumed`。
- `UnlimitedQuota=false` 时,`used_quota + 本次 > quota` 即不足。
- 用户总额度或 Key 预算任一不足 → **拒绝本次调用**(§6.5),不禁用 Key。

### 7.4 余额不足处理与提醒

- **触发**:市场服务预扣时余额不足 → 拒绝本次调用(§6.5),返回 `QUOTA_INSUFFICIENT`;不禁用 Key。
- **恢复**:无需操作。兑换/充值/管理员调额入账后,下次调用即通过预扣检查,立即恢复使用。
- **低额度提醒**:`QuotaRemindThreshold`(Option,默认 0=不提醒);成功消费后余额低于阈值,异步发邮件。

### 7.5 用户自有服务开关(D12)

| Option `UserOwnedServicesEnabled`(bool,默认 `true`) | 行为 |
|---|---|
| `true`(默认,兼容现状) | 用户可注册 `source='user'` 自有服务、创建分组、经 `/mcp` 与 `/mcp/group/:slug` 调用自有服务(**免费**) |
| `false`(纯市场模式) | 禁止 `POST /services`(创建自有服务);`/mcp`、`/mcp/group/:slug` 仅可调用**市场引用服务**(`source=marketplace`,计费) |

> 该开关**只守卫 `source=user` 自有服务**;**市场引用服务**(`source=marketplace`)始终可用(商业 offerings),不受开关影响。开关是平台策略(如"只卖平台托管服务、不允许用户自带 MCP Server"的 SaaS 模式)。

---

## 8. 兑换码(V1)

### 8.1 生成(管理员)

- `code`:`common.GetUUID()` 32 位(参考 new-api `controller/redemption.go:63-125`)。
- 支持批量(`count`,上限 100),统一 `quota` 面值与有效期。
- API:`POST /api/v1/admin/redemptions`(批量)、`GET`(分页/搜索)、`PUT`(启停)、`DELETE`。

### 8.2 兑换(用户)

- API:`POST /api/v1/redemptions/redeem` `{ "code": "..." }`。
- 流程(参考 new-api `Redeem()` [model/redemption.go:115-156](../model/redemption.go)):
  1. `RandomSleep()` 防并发碰撞。
  2. 事务内 `FOR UPDATE` 锁定兑换码。
  3. 校验 `status=1` 且未过期。
  4. `IncreaseUserQuota(user, quota)` + `total_topup += quota`。
  5. 置 `status=2`、记 `user_id`/`redeemed_at`。
  6. 写流水(类型=兑换)。
  (Key 全程不禁用,入账后立即可用,无需恢复)

---

## 9. 充值与支付(V2,设计占位)

### 9.1 支付网关抽象

```go
// service/payment.go (V2)
type PaymentGateway interface {
    CreateOrder(tradeNo string, money float64) (payURL string, err error)
    VerifyCallback(req *http.Request) (tradeNo string, money float64, ok bool)
    Provider() string  // epay / alipay / stripe
}
```

> 参考 new-api 多渠道(stripe/creem/epay/waffo)。V2 先接 1~2 种(推荐易支付 epay 覆盖支付宝/微信)。回调走**订单号幂等** + `FOR UPDATE` + 受影响行数校验(参考 `Recharge()` [model/topup.go:109-160](../model/topup.go))。

### 9.2 流程

```
用户选金额 → 创建 topup(status=pending) → 网关 CreateOrder → 跳转支付
   → 网关回调 → VerifyCallback → 校验订单 → quota = money × QuotaPerUnit → IncreaseUserQuota → status=success
   (Key 全程不禁用,无需恢复)
```

---

## 10. 订阅(V2,设计占位)

- 管理员创建 `subscription_plans`(套餐:价格、时长、内含额度、升级分组)。
- 用户购买(支付或余额)→ 创建 `user_subscriptions`(active, start/end_time, amount_total)。
- 计费时若用户有 active 订阅且 `billing_preference=subscription_first`:优先扣订阅 `amount_used`,不够再回退钱包(参考 new-api `billing_session.go`,V2 简化版)。
- 到期:`status=expired`,可选降级 `User.Group` 回 `downgrade_group`。

---

## 11. 服务市场商业化(平台托管 + 引用式安装)

| 既有能力 | 本期变化 |
|----------|----------|
| 管理员上架市场项([service/marketplace.go](../service/marketplace.go)) | **手动添加** 或 **从自有服务克隆**(**仅管理员自己账户下**的自有服务,深拷贝,无关联,凭证保留并提示替换);配置上游 transport(**平台托管**)+ **服务级定价**;**非自用模式必须显式定价**(§5.6) |
| **虚拟服务上架(禁止,D16)** | 视觉/摄像头等虚拟服务(`transport_type='virtual'`)**仅自有配置免费、不可上架**——手动添加拒 `transport_type='virtual'`、从自有服务克隆时源为虚拟服务亦拒;与定价/自用模式无关的硬约束 |
| 用户浏览市场 `GET /marketplace` | 列表/详情展示**价格**(或"免费"标记) |
| 用户添加市场服务 | **新增** `POST /marketplace/:id/add` —— 建立引用(生成 `source='marketplace'` 的 `mcp_services` 行,config 空,不复制凭证,复制 tools_cache) |
| 用户调用市场服务 | 加入分组 → 经 `/mcp/group/:slug` 统一调用;resolver 按 `source=marketplace` 用平台 session,**按市场价扣费** |
| 市场项批量定价 | **新增** `PUT /admin/marketplace/pricing/batch`(多选已上架服务批量设价) |
| ~~`POST /marketplace/install`(复制配置)~~ | **删除/替换**:旧"复制配置安装"废弃,改为引用式 `POST /marketplace/:id/add` |
| ~~`POST /mcp/marketplace/:slug`(直连端点)~~ | **不采用**:市场服务统一走分组端点,不另建端点(V1.5 决定) |
| **市场项下架/删除**(新增规则) | **下架**(`status=下架`/软删除):从浏览隐藏,**已添加用户的引用保留可用**;**硬删除**:级联清理所有用户引用(`source=marketplace` 行)+ 分组关联,需二次确认 |
| **用户重复添加去重** | 同一用户对同一市场项**仅一份引用**(唯一约束 `user_id + marketplace_item_id`);重复 `POST /marketplace/:id/add` 返回已有引用 |
| 评分 `marketplace_reviews` | 保持 |
| 源码型(`category='source'`) | 托管模型下不适用,降级为文档展示或下线 |

> **与用量计量统一**:市场引用服务调用走现有分组调用链路,计费 hook 在 resolver 命中 `source=marketplace` 时触发(§6),3 级定价解析(§5.2)。用户"添加到我的服务"后,市场服务与自有服务在分组里无差别使用,仅按来源计费。
>
> **虚拟服务不上架(D16)**:视觉/摄像头等虚拟服务是配置者私有的内置 handler,凭证/配置绑定其个人资源(如 `vision_configs`),不存在"平台托管上游"。故手动添加与从自有服务克隆两个上架入口均对 `transport_type='virtual'` 硬拒绝;虚拟服务 `source`∈{vision,camera}、配置者自己免费调用,不进入市场流通与计费。
>
> **克隆来源归属(权限边界)**:"从自有服务克隆"(`POST /admin/marketplace/clone`)与来源下拉(`GET /admin/marketplace/clone-sources`)**仅限管理员自己账户下**的自有服务(`mcp_services.user_id = 当前管理员`);**管理员不得克隆/上架其他用户的服务**。后端 `CloneFromService` 校验 `svc.UserID == adminID`,不匹配返回 `ErrServiceNotOwned`——避免管理员视野暴露其他用户私有服务或复制其私有凭证。
>
> **市场项变更的传导**:① **定价/上游配置变更**:resolver 实时读 `marketplace_items`(经价格缓存),改价/换凭证**立即对所有引用生效**(改价后 `InvalidatePricingCache`);② **工具目录变更**:V1 引用持有 `tools_cache` 快照,需用户"重新同步"或 V2 自动下发(任务 18);③ **上游健康**:平台对市场项统一健康检查/熔断,不可用时调用失败→退款(§6.6)。

---

## 12. 用量看板与统计(V2)

- **个人用量页**:消费趋势(基于 `mcp_usage_hourly`)、按市场服务/工具消费 Top、账单明细(基于 `mcp_call_logs.quota_consumed`,仅市场来源)。
- **管理员看板**:扩展 `GET /admin/stats` 增加营收/消费维度;消费排行(参考 new-api `GetRankingQuotaTotals` [model/usedata_rankings.go](../model/usedata_rankings.go))。
- 前端需引入图表库(参考 new-api default 用 recharts / VisActor VChart)。

---

## 13. API 接口设计

> 鉴权三层沿用现有([router/api_router.go](../router/api_router.go)):公开 / 用户(UserAuth) / 管理员(AdminAuth) / MCP 网关(APIKeyAuth)。响应格式同 [API.md](./API.md)。

### 13.1 MCP 网关侧(统一分组端点)

| 方法 | 端点 | 计费行为 |
|------|------|----------|
| POST | `/mcp`、`/smart/mcp`、`/mcp/group/:slug` | 统一调用路径:调用 `source=user` 服务**免费**;调用 `source=marketplace` 服务按 3 级定价**按次计费**,余额不足拒绝本次调用 + 错误(不禁用 Key) |

余额不足错误体(MCP JSON-RPC,参考 new-api HTTP 403 `ErrorCodeInsufficientUserQuota`):
```json
{ "jsonrpc":"2.0", "id":1, "error":{ "code":-32603, "message":"用户额度不足,剩余额度: ¥0.00,请充值或兑换", "data":{ "code":"QUOTA_INSUFFICIENT" } } }
```

### 13.2 用户侧(钱包/用量/兑换/市场)

| 方法 | 路径 | 说明 | 期 |
|------|------|------|----|
| GET | `/wallet` | 我的额度概览(quota/used_quota/请求次数) | V1 |
| GET | `/wallet/billing` | 消费明细(分页,基于 mcp_call_logs;仅市场来源) | V1 |
| GET | `/wallet/usage/stats` | 用量统计(今日/本周消费、按市场服务分布) | V1 |
| POST | `/redemptions/redeem` | 兑换码兑换 | V1 |
| GET | `/marketplace` | 浏览市场(含价格) | V1 |
| **POST** | **`/marketplace/:id/add`** | **添加市场服务到我的服务**(建立引用,不复制配置/凭证) | V1 |
| DELETE | `/services/:id` | 删除服务(含市场引用:仅删自己账户的引用行,不影响市场项) | V1 |
| POST | `/wallet/topup` | 发起充值(V2) | V2 |
| GET | `/wallet/topups` | 充值订单历史(V2) | V2 |
| GET | `/subscriptions/plans` | 套餐列表(V2) | V2 |

### 13.3 管理员侧(定价/兑换码/调额/统计)

| 方法 | 路径 | 说明 | 期 |
|------|------|------|----|
| PUT | `/admin/settings` | 全局计费配置 + `UserOwnedServicesEnabled` 开关(新增 key 见 §15) | V1 |
| **PUT** | **`/admin/marketplace/pricing/batch`** | **批量设置已上架市场服务价格**(D4,§5.5) | V1 |
| POST | `/admin/marketplace` | 手动添加市场项(空白表单;**非自用模式须显式定价**,§5.6) | V1 |
| POST | `/admin/marketplace/clone` | 从自有服务克隆(`from_service_id`,深拷贝,凭证保留待替换;**非自用模式须显式定价**) | V1 |
| PUT | `/admin/marketplace/:id` | 扩展:市场项服务级定价 + 上游 transport + 启用/上架(**非自用模式须显式定价**) | V1 |
| PUT | `/admin/marketplace/:id/tools/:tool/pricing` | 工具级定价(§4.4) | V2 |
| **POST** | **`/admin/users/:id/quota`** | **管理员调额**(mode=add/sub/set,参考 new-api,D13) | V1 |
| GET/POST | `/admin/users[/:id]` | 用户管理(创建可设初始 quota) | V1 |
| GET/POST/PUT/DELETE | `/admin/redemptions[/:id]` | 兑换码管理 | V1 |
| GET | `/admin/billing/pricing` | 定价总览(各市场服务价格) | V1 |
| GET | `/admin/billing/stats` | 营收/消费统计 | V2 |
| GET/POST/PUT/DELETE | `/admin/subscriptions/plans[/:id]` | 套餐管理 | V2 |

**管理员调额请求体**(`POST /admin/users/:id/quota`,参考 new-api `ManageRequest`):
```json
{ "mode": "add", "value": 500000, "remark": "补偿6月故障" }
```
> `mode`: `add`(增)/ `sub`(减)/ `set`(覆盖);`value` 单位 quota。带角色权限校验 + 审计日志(§7.2)。

---

## 14. 前端页面设计

> 技术栈:React 19 + TanStack Router/Query + shadcn/ui + zustand,feature-based 目录([web/src/features](../web/src/features))。菜单在 [web/src/components/layout/app-sidebar.tsx](../web/src/components/layout/app-sidebar.tsx),管理员守卫在 [web/src/routes/_authenticated/admin/route.tsx](../web/src/routes/_authenticated/admin/route.tsx)。

### 14.1 新增/调整页面清单

| 期 | 页面 | feature 目录 | 路由 | 菜单 |
|----|------|--------------|------|------|
| V1 | 钱包(额度+消费明细+用量) | `features/wallet/` | `/wallet` | mainNav |
| V1 | 兑换码兑换(钱包内卡片) | `features/wallet/components/` | `/wallet` | — |
| V1 | 公开价格展示(市场价/默认价) | `features/pricing/` | `/pricing`(公开) | 顶部导航 |
| V1 | 管理员-兑换码管理 | `features/redemption-codes/` | `/admin/redemption-codes` | adminNav |
| V1 | 管理员-计费设置(默认价/分组倍率/开关/自有服务开关) | `features/admin/billing/` | `/admin/billing` | adminNav |
| V1 | **管理员-用户额度调整对话框**(增/减/设) | 扩展 `features/admin/components/` | (弹窗,用户管理内) | — |
| V1 | 市场(列表/详情)展示价格 + **"添加到我的服务"按钮** | 扩展 `features/marketplace/` | `/marketplace` | 已有 |
| V1 | 市场-管理员上架(手动/**从自有服务克隆+凭证高亮提示**)+ 定价 + transport + **批量定价** | 扩展 `features/admin/marketplace/` | `/admin/marketplace` | 已有 |
| V1 | **服务列表:市场来源服务显示"市场"徽标,配置只读(平台托管)** | 扩展 `features/services/` | `/services` | 已有 |
| V1 | 调用日志展示计费列(quota/单价/状态/来源/市场项) | 扩展 `features/logs/` | `/logs` | 已有 |
| V1 | API Key 列表展示余额/预算用量 | 扩展 `features/api-keys/` | `/api-keys` | 已有 |
| V2 | 充值/支付 | `features/wallet/` | `/wallet` | — |
| V2 | 订阅套餐 | `features/subscriptions/` | `/subscriptions` `/admin/subscriptions` | mainNav+adminNav |
| V2 | 用量看板(图表) | 扩展 `features/dashboard/` | `/dashboard` | 已有 |

### 14.2 菜单接入

```ts
// web/src/components/layout/app-sidebar.tsx
mainNav 新增: { label: 'nav.wallet', icon: Wallet, href: '/wallet' }
            { label: 'nav.pricing', icon: Tag, href: '/pricing' }
adminNav 新增:{ label: 'nav.adminBilling', icon: CreditCard, href: '/admin/billing', adminOnly: true }
            { label: 'nav.adminRedemption', icon: Ticket, href: '/admin/redemption-codes', adminOnly: true }
```

### 14.3 feature 骨架(遵循 new-mcp 约定)

```
features/wallet/
  ├─ api.ts                    # 复用 @/lib/api (Bearer token, /api/v1 前缀)
  └─ components/wallet-page.tsx
features/redemption-codes/
  ├─ api.ts
  └─ components/{redemptions-page, redemptions-table, redemptions-mutate-drawer}.tsx
```

### 14.4 现有可复用基础

- 类型:`web/src/types/index.ts` 已有 `quota`/`used_quota`/`request_count`/`group`(User)、`quota`/`unlimited_quota`/`used_quota`(ApiKey),需补 billing + service.source/marketplace_item_id 类型。
- i18n:`zh.json` 已有"额度/quota""剩余额度""已用额度"键,需补计费/兑换/钱包/批量定价/市场徽标相关。
- 参考实现:new-api default 的 `features/wallet`、`features/redemption-codes`、`features/pricing`、`features/users/components/user-quota-dialog.tsx`、`features/system-settings/billing` 可直接对照(注意 new-api 用 cookie 鉴权,new-mcp 用 Bearer token,API 调用复用本地 `@/lib/api`)。

---

## 15. 配置项清单(options 新增)

> 沿用 `OptionMap` 模式([model/option.go](../model/option.go)),启动加载到内存,`GetOptionBool/Int/String` 读取。注册进 `defaultOptions`([model/option.go:22-42](../model/option.go))。

| 分类 | Key | 类型 | 默认 | 说明 |
|------|-----|------|------|------|
| 计费 | `BillingEnabled` | bool | false | 总开关,false 时市场服务也跳过计费 |
| 计费 | `QuotaPerUnit` | int | 500000 | 1 货币单位 = 多少 quota |
| 计费 | `DisplayCurrency` | string | CNY | 展示货币(CNY/USD) |
| 计费 | `BillingDefaultType` | string | per_call | 全局默认计费类型(仅 free/per_call) |
| 计费 | `BillingDefaultPricePerCall` | string(decimal) | 0 | 全局默认按次单价(市场服务第 3 级) |
| 计费 | `GroupRatio` | string(JSON) | {"default":1,"vip":1,"svip":1} | 分组倍率 |
| 计费 | `TrustQuota` | int | 5000000 | 信任额度旁路阈值(默认 10 元) |
| 计费 | `ChargeAdmin` | bool | false | 是否对管理员计费 |
| 计费 | `ChargeOnClientError` | bool | false | 客户端参数错误是否收费 |
| 计费 | `ChargeOnTimeout` | bool | false | 超时是否收费 |
| 计费 | `BillingFailOpen` | bool | true | 计费 DB 异常时是否放行(记欠账) |
| 额度 | `QuotaForNewUser` | int | 0 | 新用户赠送额度 |
| 额度 | `QuotaRemindThreshold` | int | 0 | 低额度邮件提醒阈值(0=不提醒) |
| 日志 | `LogPayloadEnabled` | bool | true | 是否落 `request/response_payload`(false 仅存元数据+计费列,省空间/隐私) |
| 日志 | `LogRetentionDays` | int | 30 | 调用日志 TTL(天),0=永久;定时清理过期 |
| **自有服务** | **`UserOwnedServicesEnabled`** | **bool** | **true** | **是否允许用户添加/调用自有服务(source=user);false=纯市场模式(市场引用仍可用)** |
| **自用模式** | **`SelfUseModeEnabled`** | **bool** | **false** | **自用模式可用全局默认价;非自用(默认)市场上架/启用必须显式定价(参考 new-api)** |
| 兑换 | `RedemptionEnabled` | bool | true | 是否开放兑换 |
| 支付(V2) | `PaymentEnabled` | bool | false | 在线支付开关 |
| 支付(V2) | `EpayEndpoint`/`EpayPID`/`EpayKey` | string | — | 易支付配置(敏感) |

> 敏感 key(支付密钥)加入 [model/option.go:44](../model/option.go) 掩码列表;公开 key(BillingEnabled、DisplayCurrency、SelfUseModeEnabled)加入 [model/option.go:48](../model/option.go) 公开列表(供 `/pricing`、市场上架页按模式适配)。

---

## 16. 分期实施路线

### V1(市场按次计费闭环)

| # | 任务 | 涉及文件 | 依赖 |
|---|------|----------|------|
| 1 | 数据模型扩展(users/marketplace_items/mcp_call_logs/mcp_tool_prices/redemptions + options;mcp_services 复用 source/marketplace_item_id) | `model/*.go`、`model/main.go:migrateDB` | — |
| 2 | 复核/改造 `DecreaseUserQuota` 为原子防透支 | `model/user.go:106` | 1 |
| 3 | 定价解析服务 `service/pricing.go`(市场 3 级解析 + 分组倍率 + 内存缓存) | `service/pricing.go` | 1 |
| 4 | 计费服务 `service/billing.go`(PreConsume/Confirm/Refund + 信任旁路 + 余额不足拒绝 + 低额度提醒 + **request_id 幂等**) | `service/billing.go` | 2,3 |
| 5 | **引用式安装** `POST /marketplace/:id/add`(创建 source=marketplace 引用行,config 空,复制 tools_cache) | `service/marketplace.go`、`controller/marketplace.go`、`router/api_router.go` | 1 |
| 6 | **resolver 改造**:`routeOrConnect` 支持 source=marketplace(平台 session,按 marketplace_item_id 复用,平台凭证);**计费 hook 接入 handleToolsCall 插入点 A/B(仅 source=marketplace)**;source=user 保持免费 | `internal/mcp/handler/gateway_handler.go`、`internal/mcp/bridge/` | 4,5 |
| 7 | 删除旧 `POST /marketplace/install`(复制配置版);`UserOwnedServicesEnabled` 开关守卫 source=user 的创建/调用 | `service/marketplace.go`、`middleware/`、`controller/service.go` | 5,6 |
| 8 | 市场项上架(手动 + **从自有服务克隆**)+ 服务级定价 + **批量定价接口** + 上游 transport 配置(**凭证加密落库**)+ **非自用模式强制定价门控**(§5.6)+ **存量项迁移**(显式定价) | `dto/marketplace.go`、`controller/marketplace.go`、`service/marketplace.go` | 1 |
| 9 | 兑换码 model/service/controller/router | `model/redemption.go` 等 | 1 |
| 10 | **管理员调额** `POST /admin/users/:id/quota`(add/sub/set + 权限 + 审计) | `controller/admin.go`、`model/user.go` | 1 |
| 11 | 系统设置计费配置项(含 `UserOwnedServicesEnabled`、`SelfUseModeEnabled`、`LogPayloadEnabled`、`LogRetentionDays`) | `service/settings.go`、`controller/settings.go` | 1 |
| 12 | 前端:钱包/兑换/计费设置/市场价+**"添加到我的服务"**/**批量定价**/**服务列表市场徽标(只读配置)**/日志计费列/**用户调额对话框**/Key余额用量 | `web/src/features/*` | 5,6,8,9,10,11 |
| 13 | 同步 [DATABASE.md](./DATABASE.md)(补齐过时列 + 新增表)、[API.md](./API.md)、[PROGRESS.md](./PROGRESS.md);调用日志 TTL 定时清理任务 | `docs/*`、`service/` | 1-12 |

### V2(资金入口 + 运营)

| # | 任务 |
|---|------|
| 14 | 充值订单 + 支付网关抽象(易支付优先)+ 回调幂等 |
| 15 | 订阅套餐 + 用户订阅 + 订阅优先计费(简化版) |
| 16 | `mcp_usage_hourly` 聚合 + 用量看板/排行(引入图表库) |
| 17 | 工具级精确定价 UI(`mcp_tool_prices` 按工具调价) |
| 18 | 市场引用服务 tools_cache 自动同步(平台工具变更下发) |
| 19 | 余额变更通用流水表 `balance_changes`(对账) |

---

## 17. 风险与边界

| 风险 | 说明 | 应对 |
|------|------|------|
| **并发透支** | 高并发下余额检查与扣减间存在窗口 | 原子 `UPDATE ... WHERE quota >= ?`;信任旁路接受有界超支 |
| **精度** | 浮点价格 × QuotaPerUnit 换算 | 落库 decimal,内存换算 `round` 成整数后只动整数 |
| **退款一致性** | 预扣成功但退款失败导致少退/多扣 | 退款幂等 + `billing_status` 状态机 + 对账任务(V2) |
| **引用服务一致性** | 用户添加的市场引用 tools_cache 与平台项脱节 | V1 添加时快照 + 手动同步;V2 自动下发(任务 18) |
| **平台 session 共享** | 同一市场项多用户共享平台 session | session 按 marketplace_item_id 复用;平台统一健康检查/熔断 |
| **批量定价并发** | 批量改价期间正在调用的请求价格快照 | 改价后 `InvalidatePricingCache` 刷新;单次调用内价格已快照到 log |
| **三库兼容** | 保留字 `group`/`key`、布尔默认值差异 | GORM map 条件 + `code` 替代 `key` + 布尔默认走代码而非 `default:1`([memory: db-reserved-words]) |
| **计费阻塞主链路** | DB 扣减失败影响调用 | 默认 **FailOpen**(放行+记欠账),可配关闭 |
| **平台凭证泄露** | marketplace_items 含平台上游 Key | 敏感字段加密落库 + API 掩码(§4.3) |
| **计费口径歧义** | 误对 tools/list 等扣费,或漏 mcp.execute | 明确仅 `tools/call`+`mcp.execute` 扣费(§6.7) |
| **市场项删除致引用悬空** | 硬删除致用户引用失效 | 软删除优先;硬删除级联清理引用(§11) |
| **重扣/幂等** | 网关异常重试重复扣费 | `request_id` 幂等键(§6.3);退款幂等 |
| **FailOpen 欠账** | 放行累积欠账未追 | `billing_status='debt'` + 预扣补扣/对账任务(§6.6) |
| **日志膨胀/隐私** | payload 含敏感数据且量大 | TTL 清理 + 脱敏 + `LogPayloadEnabled` 开关(§4.5) |
| **审计可追溯** | 计费涉及资金 | `mcp_call_logs` 留全部明细 + 管理员调额审计 + 余额变更流水(V2) |
| **测试** | 计费/账务是不变量 | 表驱动测试,`require` 断言额度守恒(扣减=消费+退款)、source 门控正确(自有免费/市场扣费);参考 new-api AGENTS 测试规范 |

---

## 附录 A:与 new-api 的映射关系

| new-mcp 概念 | new-api 对应 | 处理 |
|--------------|--------------|------|
| `User.Quota/UsedQuota` | `user.Quota/UsedQuota` | 直接复用 |
| `service/billing.go` BillingSession | `service/billing_session.go` | **大幅简化**(去 FundingSource/订阅/估算/两步提交,只精确预扣) |
| `service/pricing.go`(市场 3 级) | `model/pricing.go` + ratio_setting | 去掉所有 token 倍率,留分组倍率;3 级=工具/服务/全局 |
| 按次计费公式 | `service/tool_billing.go` | 借鉴按次定价思路,固定单价 |
| 余额不足处理 | `PreConsumeQuota` + `checkAndSendQuotaNotify` | 移植:拒绝本次调用 + 低额度提醒,不禁用 Key |
| 信任额度旁路 | `GetTrustQuota` / `PreConsumeQuota` 信任分支 | 移植 |
| **管理员调额** | `POST /api/user/manage` `add_quota`(add/sub/set) + `canManageTargetRole` + `recordManageAuditFor` | 移植:用户管理内调额 + 权限 + 审计 |
| **引用式安装(市场服务进分组)** | (无直接对应) | new-mcp 原创:市场引用(source=marketplace)加入用户分组,平台托管连接,分组路径按 source 计费 |
| 批量定价 | (无直接对应) | new-mcp 原创:管理员批量设市场服务价 |
| 市场克隆上架 | (无直接对应) | new-mcp 原创:从自有服务深拷贝上架,凭证保留并提示替换 |
| 用户自有服务免费 + 开关 | (无对应) | new-mcp 原创:自有服务不收费,`UserOwnedServicesEnabled` 开关 |
| **自用模式** | `operation_setting.SelfUseModeEnabled` + `relay/helper/price.go` 价格校验 + `controller/model.go:209` acceptUnsetRatio | 移植:自用模式可用全局默认,非自用(默认)强制定价 |
| `redemptions` | `model/redemption.go` | 移植,列名 `code` |
| `topups`(V2) | `model/topup.go` | 移植,支付渠道精简 |
| 订阅(V2) | `model/subscription.go` | 移植精简版 |
| `mcp_call_logs` 计费列 | `model/log.go` + `Other` JSON | 同行扩展,非独立表 |
| `mcp_usage_hourly`(V2) | `model/usedata.go` QuotaData | 移植聚合模式 |
| **不移植** | completion_ratio / cache_ratio / image_ratio / audio_ratio / **per_token 透传** / billingexpr 表达式引擎 / 硬编码模型倍率前缀 | token 计费特有,MCP 不适用 |

---

## 附录 B:一次分组调用的计费伪代码(source 门控)

```go
// gateway_handler.go handleToolsCall 内(伪代码)
service := resolveService(...)                       // 含 source 字段

if service.Source != "marketplace" {
    // 自有服务:免费,直接转发,不触碰 BillingService
    result := routeAndCall(service, ...)
    writeCallLog(billingStatus: "skipped", quotaConsumed: 0)
    return result
}

// 市场来源服务:计费
item := loadMarketplaceItem(service.MarketplaceItemID)
price := pricing.ResolveMarketplace(item, toolName, user.Group)   // 3 级解析 + 分组倍率

// 插入点 A:预扣
sess, err := billing.PreConsume(ctx, price)
if err == ErrInsufficientQuota {
    // 拒绝本次调用,不禁用 Key(参考 new-api PreConsumeQuota)
    writeCallLog(marketplaceItemID: item.ID, billingStatus: "blocked", quotaConsumed: 0)
    return jsonrpcError("QUOTA_INSUFFICIENT")  // HTTP 403 风格
}

// 执行上游(平台 session,按 item.ID 复用)
result, callErr := routeAndCallPlatform(item, ...)

// 插入点 B:确认/退款
if callErr != nil {
    billing.Refund(sess)                          // 全额退(Trusted 则未扣,无操作)
    writeCallLog(marketplaceItemID: item.ID, billingStatus: "refunded", quotaConsumed: 0, unitPrice: price)
} else {
    billing.Confirm(sess)                         // Trusted 补扣;否则 no-op(已扣)
    billing.MaybeNotifyLowQuota(user, sess)       // 余额低于 QuotaRemindThreshold 发邮件(异步)
    writeCallLog(marketplaceItemID: item.ID, billingStatus: "charged", quotaConsumed: sess.ConsumedQuota, unitPrice: price, priceScope: price.Scope)
}
```
