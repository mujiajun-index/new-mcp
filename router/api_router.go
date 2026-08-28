package router

import (
	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/controller"
	"github.com/mujkjk/newmcp/middleware"
)

func SetApiRouter(engine *gin.Engine) {
	api := engine.Group("/api/v1")
	api.Use(middleware.CORS())

	// Public endpoints
	api.POST("/auth/register", controller.Register)
	api.POST("/auth/login", controller.Login)
	api.GET("/setup", controller.GetSetup)
	api.POST("/setup", controller.PostSetup)

	// Public marketplace browsing
	api.GET("/marketplace", controller.BrowseMarketplace)
	api.GET("/marketplace/:id", controller.GetMarketplaceItem)
	api.GET("/marketplace-groups", controller.BrowseMarketplaceGroups) // 启用分组(广场左侧筛选)

	// Public settings (system name, footer, etc.)
	api.GET("/settings/public", controller.GetPublicSettings)

	// Email verification code (registration) — public, rate-limited per IP
	api.GET("/verification", middleware.EmailVerificationRateLimit(), controller.SendEmailVerification)

	// Camera stream (auth via query token — WebSocket can't send custom headers)
	api.GET("/cameras/:id/stream", HandleCameraStream)

	// Vision image upload for MCP clients (API-key auth + rate limit). Separate
	// path from the JWT /vision/upload because gin forbids two middleware sets
	// on one route; shares the same controller handler.
	api.POST("/vision/mcp-upload", middleware.APIKeyAuth(), middleware.RateLimit(), controller.UploadVisionImage)

	// V1.1: upload management for MCP clients (API-key auth + rate limit).
	api.GET("/vision/mcp-uploads", middleware.APIKeyAuth(), middleware.RateLimit(), controller.ListUploads)
	api.DELETE("/vision/mcp-uploads/:id", middleware.APIKeyAuth(), middleware.RateLimit(), controller.DeleteUpload)

	// User-authenticated endpoints
	auth := api.Group("")
	auth.Use(middleware.UserAuth())
	{
		auth.GET("/auth/profile", controller.GetProfile)
		auth.PUT("/auth/profile", controller.UpdateProfile)
		auth.PUT("/auth/password", controller.ChangePassword)

		// Email bind/change verification code (authenticated, rate-limited per IP)
		auth.GET("/auth/profile/email-code", middleware.EmailVerificationRateLimit(), controller.SendEmailBindVerification)

		// MCP Services
		auth.GET("/services", controller.ListServices)
		auth.POST("/services", controller.CreateService)
		auth.POST("/services/test-connection", controller.TestConnection)
		auth.POST("/services/prepare-stdio", controller.PrepareStdio)
		auth.GET("/services/:id", controller.GetService)
		auth.PUT("/services/:id", controller.UpdateService)
		auth.DELETE("/services/:id", controller.DeleteService)
		auth.POST("/services/:id/test", controller.TestService)
		auth.POST("/services/:id/refresh-tools", controller.RefreshTools)
		auth.GET("/services/:id/tools", controller.GetServiceTools)
		auth.POST("/services/:id/tools/call", controller.CallServiceTool)
		auth.GET("/services/:id/resources", controller.GetServiceResources)
		auth.POST("/services/:id/resources/read", controller.ReadServiceResource)
		auth.GET("/services/:id/prompts", controller.GetServicePrompts)
		auth.POST("/services/:id/prompts/get", controller.CallServicePrompt)
		auth.GET("/services/:id/health", controller.GetServiceHealth)
		auth.GET("/services/:id/process", controller.GetServiceProcess)
		auth.POST("/services/:id/process/control", controller.ControlServiceProcess)
		// 服务总览(所有登录用户,按 user_id 只看自己的服务):统计摘要 + stdio 进程
		// 资源快照 + 非 stdio 服务健康快照
		auth.GET("/services/overview", controller.GetServicesOverview)

		// MCP Groups
		auth.GET("/groups", controller.ListGroups)
		auth.POST("/groups", controller.CreateGroup)
		auth.GET("/groups/check-name", controller.CheckGroupName)
		auth.GET("/groups/:id", controller.GetGroup)
		auth.PUT("/groups/:id", controller.UpdateGroup)
		auth.DELETE("/groups/:id", controller.DeleteGroup)
		auth.POST("/groups/:id/services", controller.AddGroupServices)
		auth.DELETE("/groups/:id/services/:serviceId", controller.RemoveGroupService)
		auth.GET("/groups/:id/tools", controller.GetGroupTools)
		auth.PUT("/groups/:id/tools/:toolName", controller.UpdateGroupTool)
		auth.PUT("/groups/:id/tools/batch", controller.BatchUpdateGroupTools)
		auth.GET("/groups/:id/resources", controller.GetGroupResources)
		auth.PUT("/groups/:id/resources/batch", controller.BatchUpdateGroupResources)
		auth.GET("/groups/:id/prompts", controller.GetGroupPrompts)
		auth.PUT("/groups/:id/prompts/batch", controller.BatchUpdateGroupPrompts)
		auth.POST("/groups/:id/refresh", controller.RefreshGroup)
		auth.GET("/groups/:id/endpoint", controller.GetGroupEndpoint)

		// API Keys
		auth.GET("/api-keys", controller.ListApiKeys)
		auth.POST("/api-keys", controller.CreateApiKey)
		auth.PUT("/api-keys/:id", controller.UpdateApiKey)
		auth.DELETE("/api-keys/:id", controller.DeleteApiKey)
		auth.POST("/api-keys/:id/key", controller.GetApiKeyFullKey)
		auth.POST("/api-keys/batch-delete", controller.BatchDeleteApiKeys)
		auth.POST("/api-keys/batch-status", controller.BatchUpdateApiKeyStatus)

		// Cloud Connections
		auth.GET("/connections", controller.ListConnections)
		auth.POST("/connections", controller.CreateConnection)
		auth.GET("/connections/:id", controller.GetConnection)
		auth.PUT("/connections/:id", controller.UpdateConnection)
		auth.DELETE("/connections/:id", controller.DeleteConnection)
		auth.POST("/connections/:id/connect", controller.ConnectConnection)
		auth.POST("/connections/:id/disconnect", controller.DisconnectConnection)
		auth.PUT("/connections/:id/bind-apikey", controller.BindConnectionApiKey)

		// Vision Configs
		auth.GET("/vision", controller.ListVisionConfigs)
		auth.POST("/vision", controller.CreateVisionConfig)
		auth.GET("/vision/:id", controller.GetVisionConfig)
		auth.PUT("/vision/:id", controller.UpdateVisionConfig)
		auth.DELETE("/vision/:id", controller.DeleteVisionConfig)
		auth.POST("/vision/test", controller.TestVisionConfig)
		auth.POST("/vision/models", controller.ListVisionModels)
		auth.POST("/vision/:id/enable", controller.EnableVisionConfig)
		auth.POST("/vision/:id/disable", controller.DisableVisionConfig)

		// Vision image upload (web UI / JWT path) → short-lived signed URL the
		// caller passes to a vision tool's image_url. The MCP/API-key path is
		// registered separately in the public group (gin forbids two middleware
		// sets on one route) but shares the same handler.
		auth.POST("/vision/upload", controller.UploadVisionImage)

		// Cameras
		auth.GET("/cameras", controller.ListCameras)
		auth.POST("/cameras", controller.CreateCamera)
		auth.GET("/cameras/:id", controller.GetCamera)
		auth.PUT("/cameras/:id", controller.UpdateCamera)
		auth.DELETE("/cameras/:id", controller.DeleteCamera)
		auth.POST("/cameras/:id/enable", controller.EnableCamera)
		auth.POST("/cameras/:id/disable", controller.DisableCamera)
		// 推流密钥(短链接凭证)管理:查询/生成/重生成/撤销
		auth.GET("/cameras/:id/stream-key", controller.GetCameraStreamKey)
		auth.POST("/cameras/:id/stream-key", controller.GenerateCameraStreamKey)
		auth.DELETE("/cameras/:id/stream-key", controller.RevokeCameraStreamKey)
		// User logs
		auth.GET("/logs", controller.GetLogs)
		auth.GET("/logs/stats", controller.GetLogStats)

		// Marketplace user actions
		auth.POST("/marketplace/:id/add", controller.AddMarketplaceItem) // 引用式安装(空 config,平台托管)
		auth.POST("/marketplace/:id/review", controller.CreateMarketplaceReview)

		// 兑换码兑换
		auth.POST("/redemptions/redeem", controller.RedeemCode)

		// 钱包/额度(商业化)
		auth.GET("/wallet", controller.GetWallet)
		auth.GET("/wallet/usage/stats", controller.GetWalletUsageStats)

		// 邀请码/邀请奖励(对齐 new-api /api/user/aff)
		auth.GET("/invite/overview", controller.GetInviteOverview)
		auth.POST("/invite/transfer", controller.TransferAffQuota)
	}

	// Admin endpoints
	admin := api.Group("/admin")
	admin.Use(middleware.AdminAuth())
	{
		admin.GET("/users", controller.AdminListUsers)
		admin.POST("/users", controller.AdminCreateUser)
		admin.PUT("/users/:id", controller.AdminUpdateUser)
		admin.DELETE("/users/:id", controller.AdminDeleteUser)
		admin.POST("/users/:id/restore", controller.AdminRestoreUser)
		admin.POST("/users/:id/quota", controller.AdminAdjustQuota)
		admin.GET("/users/:id", controller.AdminGetUserDetail)
		admin.GET("/stats", controller.AdminGetStats)

		// Admin: Platform services
		admin.GET("/services", controller.AdminListServices)
		admin.POST("/services", controller.AdminCreateService)

		// Admin: Marketplace management(上架仅支持从自有服务克隆,无手动创建)
		admin.GET("/marketplace", controller.AdminListMarketplaceItems)
		admin.GET("/marketplace/health", controller.AdminGetMarketplaceHealth) // 平台级健康(全用户调用聚合)
		admin.POST("/marketplace/clone", controller.AdminCloneMarketplaceItem)
		admin.PUT("/marketplace/pricing/batch", controller.AdminBatchUpdateMarketplacePricing)
		admin.GET("/marketplace/clone-sources", controller.AdminListCloneSources)
		admin.GET("/marketplace/:id", controller.AdminGetMarketplaceItem)
		admin.PUT("/marketplace/:id", controller.AdminUpdateMarketplaceItem)
		admin.POST("/marketplace/:id/refresh", controller.AdminRefreshMarketplaceItem) // 手动刷新快照(平台托管项)
		admin.DELETE("/marketplace/:id", controller.AdminDeleteMarketplaceItem)

		// Admin: marketplace groups(业务分类) + tags(标签字典)
		admin.GET("/marketplace-groups", controller.AdminListMarketplaceGroups)
		admin.GET("/marketplace-groups/all", controller.AdminListAllMarketplaceGroups)
		admin.POST("/marketplace-groups", controller.AdminCreateMarketplaceGroup)
		admin.PUT("/marketplace-groups/:id", controller.AdminUpdateMarketplaceGroup)
		admin.DELETE("/marketplace-groups/:id", controller.AdminDeleteMarketplaceGroup)
		admin.GET("/marketplace-tags", controller.AdminListMarketplaceTags)
		admin.POST("/marketplace-tags", controller.AdminCreateMarketplaceTag)
		admin.PUT("/marketplace-tags/:id", controller.AdminUpdateMarketplaceTag)
		admin.DELETE("/marketplace-tags/:id", controller.AdminDeleteMarketplaceTag)

		// Admin: 兑换码管理
		admin.GET("/redemptions", controller.AdminListRedemptions)
		admin.POST("/redemptions", controller.AdminCreateRedemptions)
		admin.PUT("/redemptions/:id", controller.AdminUpdateRedemptionStatus)
		admin.DELETE("/redemptions/:id", controller.AdminDeleteRedemption)

		// Admin: System settings
		admin.GET("/settings", controller.AdminGetSettings)
		admin.PUT("/settings", controller.AdminUpdateSetting)

		// Admin: System info + update check (维护 Tab)
		admin.GET("/system/info", controller.AdminGetSystemInfo)
		admin.GET("/system/check-update", controller.AdminCheckUpdate)

		// Admin: vision upload management (list/delete any user's uploads)
		admin.GET("/vision/uploads", controller.AdminListUploads)
		admin.GET("/vision/uploads/:id/preview", controller.AdminPreviewUpload) // dry-run transform preview
		admin.DELETE("/vision/uploads/:id", controller.AdminDeleteUpload)
		admin.DELETE("/vision/uploads", controller.AdminBatchDeleteUploads) // batch delete (collection-level)
	}

	// V1.4: short vision image URLs — /u/:sid at the ROOT (not under /api/v1, to
	// stay short). Public; auth is the method-bound HMAC in ?s=, same model as the
	// legacy /api/v1/vision/files/*key routes above (kept for one release).
	engine.GET("/u/:sid", controller.GetVisionFileByShortID)
	engine.PUT("/u/:sid", controller.PutVisionFileByShortID)

	// Health check
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "version": common.Version})
	})
}
