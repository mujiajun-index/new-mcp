package dto

// --- Admin: Update marketplace item ---
// 注:市场项创建仅支持"从自有服务克隆上架"(CloneMarketplaceReq),无手动创建。

type UpdateMarketplaceItemReq struct {
	DisplayName          *string                `json:"display_name"`
	Description          *string                `json:"description"`
	IconURL              *string                `json:"icon_url"`
	Category             *string                `json:"category"`
	// 分组绑定(多对多):nil/缺省=不动;[]=清空;[...]=全量替换(同 Tags 语义)
	GroupIDs             []int64                `json:"group_ids"`
	Tags                 []string               `json:"tags"`
	Version              *string                `json:"version"`
	TransportType        *string                `json:"transport_type"`
	ConfigTemplate       map[string]interface{} `json:"config_template"`
	AuthInstructions     *string                `json:"auth_instructions"`
	RepoURL              *string                `json:"repo_url"`
	InstallGuide         *string                `json:"install_guide"`
	ConfigTemplateSource map[string]interface{} `json:"config_template_source"`
	RequiredEnv          []string               `json:"required_env"`
	ToolsSnapshot        []interface{}          `json:"tools_snapshot"`
	ResourcesSnapshot    *map[string]interface{} `json:"resources_snapshot"` // {"resources":[],"templates":[]}
	PromptsSnapshot      *[]interface{}         `json:"prompts_snapshot"`
	Status               *int                   `json:"status"`
	SortOrder            *int                   `json:"sort_order"`
	// 下载数允许管理端手工修正(展示统计,非强约束;不可为负)
	InstallCount         *int                   `json:"install_count" binding:"omitempty,gte=0"`
	// 独占进程(仅 stdio 条目生效):切换共享↔独占会踢掉该条目全部池内会话按新模式重建
	IsolatedProcess *bool `json:"isolated_process"`
	// 商业化定价:启用/上架时非自用模式须显式定价(§5.6)
	BillingType  *string  `json:"billing_type" binding:"omitempty,oneof=free per_call"`
	PricePerCall *float64 `json:"price_per_call" binding:"omitempty,gte=0"`
}

// BatchPricingReq 批量设置已上架市场服务价格(§5.5)。
type BatchPricingReq struct {
	Items []BatchPricingItem `json:"items" binding:"required,min=1,dive"`
}

type BatchPricingItem struct {
	ID           int64   `json:"id" binding:"required"`
	BillingType  string  `json:"billing_type" binding:"required,oneof=free per_call"`
	PricePerCall float64 `json:"price_per_call" binding:"gte=0"`
}

// BatchGroupsTagsReq 批量设置市场项分组/标签(替换语义)。两个独立字段各自
// 沿用 UpdateMarketplaceItemReq 口径:nil/缺省=不动;[]=清空;[...]=全量替换。
// 两者须至少传一个(全缺省=无操作,拒绝)。
type BatchGroupsTagsReq struct {
	IDs      []int64  `json:"ids" binding:"required,min=1,dive,gte=1"`
	GroupIDs []int64  `json:"group_ids"` // nil=不动;[]=清空;[...]=替换
	Tags     []string `json:"tags"`     // nil=不动;[]=清空;[...]=替换
}

// CloneMarketplaceReq 从自有服务克隆上架(§11/D14):深拷贝 transport/config/auth/tools,与源服务无关联。
type CloneMarketplaceReq struct {
	FromServiceID int64   `json:"from_service_id" binding:"required"`
	Name          string  `json:"name" binding:"required,min=1,max=128"`
	DisplayName   string  `json:"display_name"`
	Description   string  `json:"description"`
	BillingType   string  `json:"billing_type" binding:"omitempty,oneof=free per_call"`
	PricePerCall  float64 `json:"price_per_call" binding:"gte=0"`
	// 独占进程:仅源服务为 stdio 时生效(其余传输忽略置 false);默认 false=共享
	IsolatedProcess bool `json:"isolated_process"`
}

// MarketplaceEntryPrice 条目级定价(§5.2 条目维度)。Name 为条目键,与网关计费同口径:
// 工具=工具名 / 资源=上游原始 URI / 提示=上游提示名(资源模板不可定价)。
// BillingType=inherit 为显式继承服务统一价(价格须为 0,解析时回退服务级链)。
type MarketplaceEntryPrice struct {
	Kind         string  `json:"kind" binding:"required,oneof=tool resource prompt"`
	Name         string  `json:"name" binding:"required,max=512"`
	BillingType  string  `json:"billing_type" binding:"required,oneof=free per_call inherit"`
	PricePerCall float64 `json:"price_per_call" binding:"gte=0"`
}

// EntryPricingReq 全量替换某市场项的条目级定价(不在列表中的条目按缺省回退:工具→服务价,
// 资源/提示→免费;显式继承用 billing_type=inherit 行表达;空数组=清空全部条目价)。
type EntryPricingReq struct {
	Prices []MarketplaceEntryPrice `json:"prices" binding:"omitempty,dive"`
}

// MarketplaceRefreshResult 市场项快照手动刷新结果(各快照条目数)。
type MarketplaceRefreshResult struct {
	ToolsCount     int `json:"tools_count"`
	ResourcesCount int `json:"resources_count"`
	TemplatesCount int `json:"templates_count"`
	PromptsCount   int `json:"prompts_count"`
}

// MarketplaceItemHealth 管理端市场页的平台级健康:同条目下全部用户引用行的
// 真实调用聚合(20 桶 × 10 分钟,与总览被动健康同口径),含实时推导状态。
// 字段命名与总览 ServicesOverviewItem 的健康字段一致,前端共用健康色带组件。
type MarketplaceItemHealth struct {
	HealthStatus     string         `json:"health_status"`
	Buckets          []HealthBucket `json:"health_buckets"`
	LastCallAt       int64          `json:"last_call_at"`
	LastErrorMessage string         `json:"last_error_message"`
	LastErrorAt      int64          `json:"last_error_at"`
}

// MarketplaceItemProcess 管理端市场详情(stdio 条目)的进程视图:共享条目=平台唯一
// 子进程(Shared);独占条目=安装引用行**分页**枚举(Instances 为当前页,含未运行行
// 的固定形态)+ 全量运行实例的资源概述(不随分页/筛选变化)。万级安装时一次只回
// 一页,用户名只对当前页反查。
type MarketplaceItemProcess struct {
	Isolated bool                             `json:"isolated"`
	Shared   *ServiceProcessStat              `json:"shared,omitempty"`
	// 独占模式:当前页实例
	Instances []MarketplaceItemProcessInstance `json:"instances,omitempty"`
	// 独占模式:全部运行实例的资源概述(运行数/进程合计/RSS 合计/CPU 合计,多核可超 100%)
	RunningInstances int     `json:"running_instances"`
	TotalProcesses   int     `json:"total_processes"`
	MemoryBytes      int64   `json:"memory_bytes"`
	CPUPercentTotal  float64 `json:"cpu_percent_total"`
	// 独占模式:分页元数据(筛选后的安装引用行总数与当前页)
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
}

// MarketplaceItemProcessInstance 独占条目下某个安装用户的进程实例(引用行粒度)。
type MarketplaceItemProcessInstance struct {
	ServiceID int64              `json:"service_id"`
	UserID    int64              `json:"user_id"`
	Username  string             `json:"username"`
	Name      string             `json:"name"`
	// 引用行启用状态(禁用行不可拉起进程)
	Status int                `json:"status"`
	Stat   ServiceProcessStat `json:"stat"`
}

// MarketplaceProcessControlReq 条目进程操作:共享模式忽略 ServiceID(操作平台唯一
// 进程);独占模式必填(目标安装用户的引用行)。
type MarketplaceProcessControlReq struct {
	Action    string `json:"action" binding:"required,oneof=start stop restart"`
	ServiceID int64  `json:"service_id"`
}

type MarketplaceListItem struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	DisplayName   string   `json:"display_name"`
	Description   string   `json:"description"`
	IconURL       string   `json:"icon_url"`
	Category      string   `json:"category"`
	GroupIDs      []int64  `json:"group_ids"`
	GroupNames    []string `json:"group_names"`
	Tags          []string `json:"tags"`
	Version       string   `json:"version"`
	TransportType string   `json:"transport_type"`
	InstallCount  int      `json:"install_count"`
	RatingAvg     float64  `json:"rating_avg"`
	RatingCount   int      `json:"rating_count"`
	Status        int      `json:"status"`
	SortOrder     int      `json:"sort_order"`
	CreatedAt     string   `json:"created_at"`
	// 商业化定价(供市场列表展示价格/免费标记)
	BillingType  string  `json:"billing_type"`
	PricePerCall float64 `json:"price_per_call"`
}

type MarketplaceDetail struct {
	ID                   int64                  `json:"id"`
	Name                 string                 `json:"name"`
	DisplayName          string                 `json:"display_name"`
	Description          string                 `json:"description"`
	IconURL              string                 `json:"icon_url"`
	Category             string                 `json:"category"`
	GroupIDs             []int64                `json:"group_ids"`
	GroupNames           []string               `json:"group_names"`
	Tags                 []string               `json:"tags"`
	Version              string                 `json:"version"`
	TransportType        string                 `json:"transport_type"`
	// 独占进程(仅 stdio 条目有意义):false=共享(全部安装用户共用平台子进程)
	IsolatedProcess      bool                   `json:"isolated_process"`
	// 平台上游连接配置(config_template 解密):url/command/args 等结构明文,headers/env 的凭证值
	// 为首尾掩码(如 sk-A...x9z),明文凭证不离开服务端。仅 admin 详情(GetItemByID)回传供编辑,
	// 保存时掩码原样传回由后端回填明文;公开浏览(GetPublished)绝不携带。
	ConfigTemplate       map[string]interface{} `json:"config_template,omitempty"`
	ConfigTemplateSource map[string]interface{} `json:"config_template_source"`
	AuthInstructions     string                 `json:"auth_instructions"`
	RepoURL              string                 `json:"repo_url"`
	InstallGuide         string                 `json:"install_guide"`
	RequiredEnv          []string               `json:"required_env"`
	InstallCount         int                    `json:"install_count"`
	// 广场排序值:大者靠前,相同时按 install_count 降序
	SortOrder            int                    `json:"sort_order"`
	RatingAvg            float64                `json:"rating_avg"`
	RatingCount          int                    `json:"rating_count"`
	ToolsSnapshot        []interface{}          `json:"tools_snapshot"`
	ResourcesSnapshot    map[string]interface{} `json:"resources_snapshot"` // {"resources":[],"templates":[]}
	PromptsSnapshot      []interface{}          `json:"prompts_snapshot"`
	// 上游握手信息(克隆/刷新时捕获):真实 serverInfo 与协商协议版本
	ServerInfo      map[string]interface{} `json:"server_info"`
	ProtocolVersion string                 `json:"protocol_version"`
	Status               int                    `json:"status"`
	CreatedAt            string                 `json:"created_at"`
	UpdatedAt            string                 `json:"updated_at"`
	// 商业化定价
	BillingType  string  `json:"billing_type"`
	PricePerCall float64 `json:"price_per_call"`
	// 条目级定价(工具/资源/提示;仅 enabled 行)。admin 详情与公开详情共用,
	// 供管理端编辑回显与用户侧条目价格展示。
	EntryPrices []MarketplaceEntryPrice `json:"entry_prices"`
	// 条目级多秘钥(random|polling;空=单秘钥)。仅 admin 详情(GetItemByID)填充,
	// 公开浏览(GetPublished)不携带;管理页徽章与秘钥卡片可见性判断用。
	KeyMode    string `json:"key_mode,omitempty"`
	KeyCount   int    `json:"key_count,omitempty"`
	KeyEnabled int    `json:"key_enabled,omitempty"`
}

// --- User: Install from marketplace ---

type InstallFromMarketplaceReq struct {
	ItemID      int64  `json:"item_id" binding:"required"`
	NameOverride string `json:"name_override"`
}

type InstallResult struct {
	ServiceID int64  `json:"service_id"`
	Name      string `json:"name"`
}

// --- User: Rate/Review ---

type CreateReviewReq struct {
	ItemID     int64  `json:"item_id" binding:"required"`
	Rating     int    `json:"rating" binding:"required,min=1,max=5"`
	ReviewText string `json:"review_text" binding:"max=1000"`
}
