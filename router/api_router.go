package router

import (
	"github.com/gin-gonic/gin"
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

	// User-authenticated endpoints
	auth := api.Group("")
	auth.Use(middleware.UserAuth())
	{
		auth.GET("/auth/profile", controller.GetProfile)
		auth.PUT("/auth/profile", controller.UpdateProfile)
		auth.PUT("/auth/password", controller.ChangePassword)

		// MCP Services
		auth.GET("/services", controller.ListServices)
		auth.POST("/services", controller.CreateService)
		auth.POST("/services/test-connection", controller.TestConnection)
		auth.GET("/services/:id", controller.GetService)
		auth.PUT("/services/:id", controller.UpdateService)
		auth.DELETE("/services/:id", controller.DeleteService)
		auth.POST("/services/:id/test", controller.TestService)
		auth.POST("/services/:id/refresh-tools", controller.RefreshTools)
		auth.GET("/services/:id/tools", controller.GetServiceTools)
		auth.GET("/services/:id/health", controller.GetServiceHealth)

		// MCP Groups
		auth.GET("/groups", controller.ListGroups)
		auth.POST("/groups", controller.CreateGroup)
		auth.GET("/groups/:id", controller.GetGroup)
		auth.PUT("/groups/:id", controller.UpdateGroup)
		auth.DELETE("/groups/:id", controller.DeleteGroup)
		auth.POST("/groups/:id/services", controller.AddGroupServices)
		auth.DELETE("/groups/:id/services/:serviceId", controller.RemoveGroupService)
		auth.GET("/groups/:id/tools", controller.GetGroupTools)
		auth.PUT("/groups/:id/tools/:toolName", controller.UpdateGroupTool)
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

		// User logs
		auth.GET("/logs", controller.GetUserLogs)
		auth.GET("/logs/stats", controller.GetUserLogStats)

		// Marketplace user actions
		auth.POST("/marketplace/install", controller.InstallFromMarketplace)
		auth.POST("/marketplace/:id/review", controller.CreateMarketplaceReview)
	}

	// Admin endpoints
	admin := api.Group("/admin")
	admin.Use(middleware.AdminAuth())
	{
		admin.GET("/users", controller.AdminListUsers)
		admin.POST("/users", controller.AdminCreateUser)
		admin.PUT("/users/:id", controller.AdminUpdateUser)
		admin.GET("/stats", controller.AdminGetStats)
		admin.GET("/logs", controller.AdminGetLogs)
		admin.GET("/logs/stats", controller.AdminGetLogStats)

		// Admin: Platform services
		admin.GET("/services", controller.AdminListServices)
		admin.POST("/services", controller.AdminCreateService)

		// Admin: Marketplace management
		admin.GET("/marketplace", controller.AdminListMarketplaceItems)
		admin.POST("/marketplace", controller.AdminCreateMarketplaceItem)
		admin.GET("/marketplace/:id", controller.AdminGetMarketplaceItem)
		admin.PUT("/marketplace/:id", controller.AdminUpdateMarketplaceItem)
		admin.DELETE("/marketplace/:id", controller.AdminDeleteMarketplaceItem)
	}

	// Health check
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "version": "v1.0.0"})
	})
}
