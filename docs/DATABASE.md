# NewMCP 数据库设计文档

> 版本: V1.0 | 状态: 草案 | 更新日期: 2026-05-03

## 1. 概述

- 默认使用 SQLite（零配置），可选 MySQL >= 5.7.8 或 PostgreSQL >= 9.6
- 使用 GORM 作为 ORM，通过 `db.AutoMigrate()` 自动迁移
- 支持软删除（`deleted_at` 字段）

---

## 2. 完整建表 SQL (MySQL 语法)

### 2.1 users - 用户表

```sql
CREATE TABLE `users` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `username`   VARCHAR(64)     NOT NULL COMMENT '用户名，唯一',
    `password`   VARCHAR(255)    NOT NULL COMMENT '密码 (bcrypt hash)',
    `email`      VARCHAR(255)    DEFAULT '' COMMENT '邮箱',
    `role`       VARCHAR(32)     DEFAULT 'user' COMMENT '角色: admin, user',
    `status`     TINYINT         DEFAULT 1 COMMENT '状态: 1=启用, 2=禁用',
    `avatar_url` VARCHAR(512)    DEFAULT '' COMMENT '头像 URL',
    `created_at` DATETIME        DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME        DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at` DATETIME        DEFAULT NULL COMMENT '软删除时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_username` (`username`),
    KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户表';
```

### 2.2 api_keys - API 密钥表

```sql
CREATE TABLE `api_keys` (
    `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id`      BIGINT UNSIGNED NOT NULL COMMENT '所属用户 ID',
    `name`         VARCHAR(128)    NOT NULL COMMENT '密钥名称',
    `key_hash`     VARCHAR(255)    NOT NULL COMMENT 'API Key SHA256 hash',
    `key_prefix`   VARCHAR(16)     NOT NULL COMMENT 'Key 前缀 (前8位，用于识别)',
    `permissions`  TEXT            DEFAULT '{}' COMMENT '权限 JSON: {"groups":["*"],"max_rate":100}',
    `status`       TINYINT         DEFAULT 1 COMMENT '1=启用, 2=禁用',
    `expires_at`   DATETIME        DEFAULT NULL COMMENT '过期时间',
    `last_used_at` DATETIME        DEFAULT NULL COMMENT '最后使用时间',
    `created_at`   DATETIME        DEFAULT CURRENT_TIMESTAMP,
    `updated_at`   DATETIME        DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at`   DATETIME        DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_key_hash` (`key_hash`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_key_prefix` (`key_prefix`),
    KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='API 密钥表';
```

### 2.3 mcp_services - MCP 服务注册表

```sql
CREATE TABLE `mcp_services` (
    `id`               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id`          BIGINT UNSIGNED NOT NULL COMMENT '所属用户 ID',
    `name`             VARCHAR(128)    NOT NULL COMMENT '服务标识 (用户内唯一)',
    `display_name`     VARCHAR(255)    DEFAULT '' COMMENT '显示名称',
    `description`      TEXT            DEFAULT '' COMMENT '服务描述',
    `transport_type`   VARCHAR(32)     NOT NULL COMMENT '传输类型: stdio, sse, streamable-http, websocket',

    -- 连接配置 (JSON)
    -- stdio:        {"command":"npx","args":["-y","@exa/mcp"],"env":{"EXA_API_KEY":"xxx"}}
    -- sse:          {"url":"http://localhost:3000/sse","headers":{}}
    -- streamable-http: {"url":"http://localhost:3000/mcp","headers":{}}
    -- websocket:    {"url":"wss://api.xiaozhi.me/mcp/?token=eyJ...","headers":{}}
    `config`           TEXT            NOT NULL DEFAULT '{}' COMMENT '连接配置 JSON',

    -- 认证配置
    `auth_type`        VARCHAR(32)     DEFAULT 'none' COMMENT '认证类型: none, api_key, bearer, basic, oauth',
    `auth_config`      TEXT            DEFAULT '{}' COMMENT '认证配置 JSON',

    -- 缓存
    `tools_cache`      MEDIUMTEXT      DEFAULT '[]' COMMENT '工具目录缓存 JSON 数组',
    `tools_updated_at` DATETIME        DEFAULT NULL COMMENT '工具缓存更新时间',

    -- 健康状态
    `health_status`    VARCHAR(16)     DEFAULT 'unknown' COMMENT '健康状态: healthy, unhealthy, unknown',
    `last_health_check` DATETIME       DEFAULT NULL COMMENT '上次健康检查时间',

    -- 元数据
    `server_info`      TEXT            DEFAULT '{}' COMMENT '服务器信息 JSON: {name, version}',
    `protocol_version` VARCHAR(32)     DEFAULT '' COMMENT 'MCP 协议版本',
    `icon_url`         VARCHAR(512)    DEFAULT '' COMMENT '图标 URL',
    `tags`             VARCHAR(512)    DEFAULT '' COMMENT '标签 (逗号分隔)',
    `visibility`       VARCHAR(16)     DEFAULT 'private' COMMENT '可见性: private, public',
    `sort_order`       INT             DEFAULT 0 COMMENT '排序权重',
    `status`           TINYINT         DEFAULT 1 COMMENT '1=启用, 2=禁用',
    `created_at`       DATETIME        DEFAULT CURRENT_TIMESTAMP,
    `updated_at`       DATETIME        DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at`       DATETIME        DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_user_name` (`user_id`, `name`),
    KEY `idx_transport_type` (`transport_type`),
    KEY `idx_health_status` (`health_status`),
    KEY `idx_visibility` (`visibility`),
    KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='MCP 服务注册表';
```

### 2.4 mcp_groups - MCP 分组表

```sql
CREATE TABLE `mcp_groups` (
    `id`                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id`           BIGINT UNSIGNED NOT NULL COMMENT '所属用户 ID',
    `name`              VARCHAR(128)    NOT NULL COMMENT '分组标识 (用户内唯一)',
    `display_name`      VARCHAR(255)    DEFAULT '' COMMENT '显示名称',
    `description`       TEXT            DEFAULT '' COMMENT '分组描述',
    `icon_url`          VARCHAR(512)    DEFAULT '' COMMENT '图标 URL',

    -- 分组设置
    `visibility`        VARCHAR(16)     DEFAULT 'private' COMMENT '可见性: private, public',
    `auto_discover`     TINYINT         DEFAULT 1 COMMENT '自动发现成员服务的工具',

    -- MCP 端点配置
    `endpoint_slug`     VARCHAR(128)    DEFAULT '' COMMENT 'URL 路径标识 (全局唯一)',
    `endpoint_auth`     VARCHAR(32)     DEFAULT 'api_key' COMMENT '端点认证: api_key, jwt, none',

    -- 中间件配置 (V2 扩展)
    `middleware_config`  TEXT            DEFAULT '{}' COMMENT '中间件配置 JSON',

    `status`            TINYINT         DEFAULT 1 COMMENT '1=启用, 2=禁用',
    `sort_order`        INT             DEFAULT 0 COMMENT '排序权重',
    `created_at`        DATETIME        DEFAULT CURRENT_TIMESTAMP,
    `updated_at`        DATETIME        DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at`        DATETIME        DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_endpoint_slug` (`endpoint_slug`),
    UNIQUE KEY `idx_user_name` (`user_id`, `name`),
    KEY `idx_visibility` (`visibility`),
    KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='MCP 分组表';
```

### 2.5 mcp_group_services - 分组-服务关联表

```sql
CREATE TABLE `mcp_group_services` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `group_id`   BIGINT UNSIGNED NOT NULL COMMENT '分组 ID',
    `service_id` BIGINT UNSIGNED NOT NULL COMMENT '服务 ID',
    `enabled`    TINYINT         DEFAULT 1 COMMENT '是否启用',
    `sort_order` INT             DEFAULT 0 COMMENT '排序权重',
    `created_at` DATETIME        DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_group_service` (`group_id`, `service_id`),
    KEY `idx_service_id` (`service_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分组-服务关联表';
```

### 2.6 mcp_group_tools - 分组工具过滤表

```sql
CREATE TABLE `mcp_group_tools` (
    `id`                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `group_id`            BIGINT UNSIGNED NOT NULL COMMENT '分组 ID',
    `service_id`          BIGINT UNSIGNED NOT NULL COMMENT '服务 ID',
    `tool_name`           VARCHAR(255)    NOT NULL COMMENT '原始工具名',
    `enabled`             TINYINT         DEFAULT 1 COMMENT '是否启用',
    `name_override`       VARCHAR(255)    DEFAULT '' COMMENT '自定义工具名',
    `description_override` TEXT           DEFAULT '' COMMENT '自定义描述',
    `annotations`         TEXT            DEFAULT '{}' COMMENT '自定义 MCP 注解 JSON',
    `created_at`          DATETIME        DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_group_service_tool` (`group_id`, `service_id`, `tool_name`),
    KEY `idx_group_id` (`group_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分组工具过滤表';
```

### 2.7 vision_configs - 视觉模型配置表

```sql
CREATE TABLE `vision_configs` (
    `id`                   BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id`              BIGINT UNSIGNED NOT NULL COMMENT '所属用户 ID',
    `name`                 VARCHAR(128)    NOT NULL COMMENT '配置名称',
    `description`          TEXT            DEFAULT '' COMMENT '描述',

    -- 模型配置
    `provider`             VARCHAR(32)     NOT NULL COMMENT '提供商: openai, ollama, custom, glm, qwen',
    `model_name`           VARCHAR(128)    DEFAULT '' COMMENT '模型名称: gpt-4o, glm-4v, qwen-vl',
    `endpoint_url`         VARCHAR(512)    DEFAULT '' COMMENT 'API 端点 URL',
    `api_key`              VARCHAR(512)    DEFAULT '' COMMENT 'API Key (加密存储)',
    `system_prompt`        TEXT            DEFAULT '' COMMENT '系统提示词',
    `max_tokens`           INT             DEFAULT 4096 COMMENT '最大输出 tokens',

    -- MCP 集成
    `auto_register`        TINYINT         DEFAULT 1 COMMENT '自动注册为 MCP 服务',
    `registered_service_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '注册的 MCP 服务 ID',

    `status`               TINYINT         DEFAULT 1 COMMENT '1=启用, 2=禁用',
    `created_at`           DATETIME        DEFAULT CURRENT_TIMESTAMP,
    `updated_at`           DATETIME        DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at`           DATETIME        DEFAULT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_provider` (`provider`),
    KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='视觉模型配置表';
```

### 2.8 cameras - 摄像头表

```sql
CREATE TABLE `cameras` (
    `id`                   BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id`              BIGINT UNSIGNED NOT NULL COMMENT '所属用户 ID',
    `name`                 VARCHAR(128)    NOT NULL COMMENT '摄像头名称',
    `description`          TEXT            DEFAULT '' COMMENT '描述',

    -- 摄像头源
    `source_type`          VARCHAR(32)     NOT NULL COMMENT '源类型: http_mjpeg, rtsp, file, v4l2',
    `source_url`           VARCHAR(512)    NOT NULL COMMENT '源 URL 或设备路径',

    -- 帧捕获配置
    `fps`                  DECIMAL(4,1)    DEFAULT 1.0 COMMENT '捕获帧率 (FPS)',
    `resolution_w`         INT             DEFAULT 640 COMMENT '分辨率宽度',
    `resolution_h`         INT             DEFAULT 480 COMMENT '分辨率高度',

    -- 绑定视觉配置
    `vision_config_id`     BIGINT UNSIGNED DEFAULT NULL COMMENT '绑定的视觉配置 ID',

    -- MCP 集成
    `auto_register`        TINYINT         DEFAULT 1 COMMENT '自动注册为 MCP 服务',
    `registered_service_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '注册的 MCP 服务 ID',

    `status`               TINYINT         DEFAULT 1 COMMENT '1=运行中, 2=暂停, 3=错误',
    `last_capture_at`      DATETIME        DEFAULT NULL COMMENT '上次捕获时间',
    `created_at`           DATETIME        DEFAULT CURRENT_TIMESTAMP,
    `updated_at`           DATETIME        DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at`           DATETIME        DEFAULT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_vision_config` (`vision_config_id`),
    KEY `idx_status` (`status`),
    KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='摄像头表';
```

### 2.9 xiaozhi_endpoints - 小智 WSS 端点配置表

```sql
CREATE TABLE `xiaozhi_endpoints` (
    `id`                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id`           BIGINT UNSIGNED NOT NULL COMMENT '所属用户 ID',
    `name`              VARCHAR(128)    NOT NULL COMMENT '端点名称 (如"客厅小智")',

    -- 小智云 Agent 信息 (从 JWT 解析)
    `agent_id_ext`      VARCHAR(128)    DEFAULT '' COMMENT '小智 Agent ID',

    -- WSS 连接配置
    `wss_url`           VARCHAR(1024)   NOT NULL COMMENT '完整 WSS URL (含 token)',
    `token_expires_at`  DATETIME        DEFAULT NULL COMMENT 'JWT 过期时间',

    -- 绑定分组 (此端点暴露该分组内的所有工具给小智设备)
    `group_id`          BIGINT UNSIGNED DEFAULT NULL COMMENT '绑定的 MCP 分组 ID',

    -- 连接状态
    `auto_connect`      TINYINT         DEFAULT 1 COMMENT '1=自动连接, 0=手动连接',
    `connection_status`  VARCHAR(16)    DEFAULT 'disconnected' COMMENT 'connected, disconnected, error',
    `last_connected_at` DATETIME        DEFAULT NULL COMMENT '最后连接时间',
    `last_error`        TEXT            DEFAULT '' COMMENT '最近一次连接错误信息',

    `status`            TINYINT         DEFAULT 1 COMMENT '1=启用, 2=禁用',
    `created_at`        DATETIME        DEFAULT CURRENT_TIMESTAMP,
    `updated_at`        DATETIME        DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at`        DATETIME        DEFAULT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_group_id` (`group_id`),
    KEY `idx_connection_status` (`connection_status`),
    KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='小智 WSS 端点配置表';
```

### 2.10 mcp_call_logs - MCP 调用日志表

```sql
CREATE TABLE `mcp_call_logs` (
    `id`               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id`          BIGINT UNSIGNED DEFAULT NULL COMMENT '用户 ID',
    `api_key_id`       BIGINT UNSIGNED DEFAULT NULL COMMENT 'API Key ID',
    `device_id`        BIGINT UNSIGNED DEFAULT NULL COMMENT '设备 ID',
    `group_id`         BIGINT UNSIGNED DEFAULT NULL COMMENT '分组 ID',
    `service_id`       BIGINT UNSIGNED DEFAULT NULL COMMENT '上游服务 ID',

    `tool_name`        VARCHAR(255)    NOT NULL COMMENT '工具名 (namespaced)',
    `method`           VARCHAR(64)     DEFAULT '' COMMENT 'MCP 方法: tools/call, tools/list',

    `request_payload`  MEDIUMTEXT      DEFAULT NULL COMMENT '请求体 JSON',
    `response_status`  VARCHAR(16)     DEFAULT '' COMMENT '响应状态: success, error',
    `response_payload` MEDIUMTEXT      DEFAULT NULL COMMENT '响应体 JSON',

    `duration_ms`      INT             DEFAULT 0 COMMENT '耗时 (毫秒)',
    `error_message`    TEXT            DEFAULT '' COMMENT '错误信息',

    `client_ip`        VARCHAR(64)     DEFAULT '' COMMENT '客户端 IP',
    `user_agent`       VARCHAR(512)    DEFAULT '' COMMENT 'User-Agent',

    `created_at`       DATETIME        DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_created_at` (`created_at`),
    KEY `idx_tool_name` (`tool_name`),
    KEY `idx_service_id` (`service_id`),
    KEY `idx_group_id` (`group_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='MCP 调用日志表';
```

---

## 3. ER 关系图

```
users (1) ──< (N) api_keys
users (1) ──< (N) mcp_services
users (1) ──< (N) mcp_groups
users (1) ──< (N) vision_configs
users (1) ──< (N) cameras
users (1) ──< (N) xiaozhi_endpoints

mcp_groups (1) ──< (N) mcp_group_services >── (1) mcp_services
mcp_groups (1) ──< (N) mcp_group_tools   >── (1) mcp_services

mcp_groups (1) ──< (N) xiaozhi_endpoints

vision_configs (1) ──< (N) cameras

mcp_services (1) ──< (N) mcp_call_logs
mcp_groups   (1) ──< (N) mcp_call_logs
```

---

## 4. 索引策略

| 场景 | 索引 | 说明 |
|------|------|------|
| 用户登录 | `users.username` UNIQUE | 快速查找用户 |
| API Key 认证 | `api_keys.key_hash` UNIQUE | O(1) 查找 |
| 服务列表 | `mcp_services(user_id, name)` UNIQUE | 按用户过滤 |
| 分组工具聚合 | `mcp_group_services(group_id)` | 查询分组内服务 |
| 调用日志查询 | `mcp_call_logs(user_id, created_at)` | 按时间范围查询 |
| 设备认证 | `devices.device_token` UNIQUE | 快速设备查找 |
